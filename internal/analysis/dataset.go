// Package analysis provides bounded exploratory datasets and decision evaluation.
// These datasets do not replace the independently reconciled audit population.
package analysis

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	MaxRows    = 10000
	MaxColumns = 200
	MaxBytes   = 16 * 1024 * 1024
)

type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Rows retain original cell text. Type conversion occurs explicitly at evaluation.
type Dataset struct {
	Columns  []Column         `json:"columns"`
	Rows     []map[string]any `json:"rows"`
	RowCount int              `json:"row_count"`
}

var numberPattern = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)

func ParseCSV(ctx context.Context, data string) (Dataset, error) {
	if len(data) > MaxBytes {
		return Dataset{}, errors.New("file exceeds 16 MiB")
	}
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(data, "\ufeff")))
	headers, err := r.Read()
	if err != nil {
		return Dataset{}, errors.New("CSV needs a header row")
	}
	if err = validHeaders(headers); err != nil {
		return Dataset{}, err
	}
	rows := make([][]string, 0)
	for {
		if err = ctx.Err(); err != nil {
			return Dataset{}, err
		}
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Dataset{}, fmt.Errorf("CSV row %d has an invalid shape", len(rows)+1)
		}
		if len(rows) == MaxRows {
			return Dataset{}, errors.New("dataset exceeds 10,000 rows; split the file before importing")
		}
		rows = append(rows, row)
	}
	return FromRows(ctx, headers, rows)
}

func validHeaders(headers []string) error {
	if len(headers) == 0 || len(headers) > MaxColumns {
		return errors.New("select between 1 and 200 columns")
	}
	seen := map[string]bool{}
	for _, name := range headers {
		if strings.TrimSpace(name) == "" || len(name) > 256 || strings.ContainsAny(name, "\x00\r\n") || seen[name] {
			return errors.New("column names must be nonempty, unique and no longer than 256 characters")
		}
		seen[name] = true
	}
	return nil
}

func FromRows(ctx context.Context, headers []string, rows [][]string) (Dataset, error) {
	if err := validHeaders(headers); err != nil {
		return Dataset{}, err
	}
	if len(rows) == 0 || len(rows) > MaxRows {
		return Dataset{}, errors.New("dataset must contain between 1 and 10,000 rows")
	}
	d := Dataset{Columns: make([]Column, len(headers)), Rows: make([]map[string]any, 0, len(rows)), RowCount: len(rows)}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return Dataset{}, err
		}
		if len(row) != len(headers) {
			return Dataset{}, errors.New("row does not match the column count")
		}
		values := make(map[string]any, len(headers))
		for j, name := range headers {
			values[name] = row[j]
		}
		d.Rows = append(d.Rows, values)
	}
	for i, name := range headers {
		d.Columns[i] = Column{Name: name, Type: inferType(d.Rows, name)}
	}
	return d, ValidateDataset(d)
}

// ParseJSON accepts a flat array of objects and preserves JSON number spellings.
func ParseJSON(ctx context.Context, data []byte) (Dataset, error) {
	if len(data) > MaxBytes {
		return Dataset{}, errors.New("file exceeds 16 MiB")
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	var rows []map[string]any
	if err := dec.Decode(&rows); err != nil {
		return Dataset{}, errors.New("JSON must be an array of flat row objects")
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return Dataset{}, errors.New("unexpected data after JSON array")
	}
	if len(rows) == 0 || len(rows) > MaxRows {
		return Dataset{}, errors.New("dataset must contain between 1 and 10,000 rows")
	}
	keys := map[string]bool{}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return Dataset{}, err
		}
		if row == nil {
			return Dataset{}, errors.New("each JSON row must be an object")
		}
		for k, v := range row {
			keys[k] = true
			switch value := v.(type) {
			case string, nil:
			case bool:
				row[k] = strconv.FormatBool(value)
			case json.Number:
				row[k] = value.String()
			default:
				return Dataset{}, fmt.Errorf("column %q contains nested data; use flat objects", k)
			}
		}
	}
	headers := make([]string, 0, len(keys))
	for key := range keys {
		headers = append(headers, key)
	}
	sort.Strings(headers)
	if err := validHeaders(headers); err != nil {
		return Dataset{}, err
	}
	d := Dataset{Rows: rows, RowCount: len(rows)}
	for _, name := range headers {
		for _, row := range rows {
			if _, ok := row[name]; !ok {
				row[name] = nil
			}
		}
		d.Columns = append(d.Columns, Column{Name: name, Type: inferType(rows, name)})
	}
	return d, ValidateDataset(d)
}

func inferType(rows []map[string]any, name string) string {
	boolean, number, present := true, true, false
	for _, row := range rows {
		v := row[name]
		if v == nil || v == "" {
			continue
		}
		present = true
		s := fmt.Sprint(v)
		if s != "true" && s != "false" {
			boolean = false
		}
		if !numberPattern.MatchString(s) {
			number = false
			continue
		}
		// Preserve identifiers and values whose precision exceeds ordinary rule numbers.
		digits := strings.TrimLeft(strings.ReplaceAll(strings.Split(strings.ToLower(s), "e")[0], ".", ""), "-0")
		f, err := strconv.ParseFloat(s, 64)
		if err != nil || math.IsInf(f, 0) || len(digits) > 15 {
			number = false
		}
	}
	if present && boolean {
		return "boolean"
	}
	if present && number {
		return "number"
	}
	return "string"
}

func ValidateDataset(d Dataset) error {
	if len(d.Rows) == 0 || len(d.Rows) > MaxRows || d.RowCount != len(d.Rows) {
		return errors.New("dataset row count is invalid")
	}
	headers := make([]string, len(d.Columns))
	allowed := map[string]bool{}
	for i, c := range d.Columns {
		headers[i] = c.Name
		allowed[c.Name] = true
		if c.Type != "string" && c.Type != "number" && c.Type != "boolean" {
			return fmt.Errorf("column %q has an unsupported type", c.Name)
		}
	}
	if err := validHeaders(headers); err != nil {
		return err
	}
	for i, row := range d.Rows {
		if len(row) != len(d.Columns) {
			return fmt.Errorf("row %d does not match the column count", i+1)
		}
		for k, v := range row {
			if !allowed[k] {
				return fmt.Errorf("row %d contains an unknown column", i+1)
			}
			switch value := v.(type) {
			case nil, string, bool, json.Number:
			case float64:
				if math.IsNaN(value) || math.IsInf(value, 0) {
					return errors.New("dataset contains an invalid number")
				}
			default:
				return errors.New("dataset cells must be text, numbers, booleans or null")
			}
		}
	}
	b, err := json.Marshal(d)
	if err != nil || len(b) > MaxBytes {
		return errors.New("dataset exceeds 16 MiB after import")
	}
	return nil
}

func SelectColumns(d Dataset, columns []Column) (Dataset, error) {
	if err := ValidateDataset(d); err != nil {
		return Dataset{}, err
	}
	available := map[string]bool{}
	for _, c := range d.Columns {
		available[c.Name] = true
	}
	headers := make([]string, len(columns))
	for i, c := range columns {
		headers[i] = c.Name
		if !available[c.Name] {
			return Dataset{}, fmt.Errorf("column %q is not in this dataset", c.Name)
		}
	}
	if err := validHeaders(headers); err != nil {
		return Dataset{}, err
	}
	out := Dataset{Columns: columns, Rows: make([]map[string]any, len(d.Rows)), RowCount: d.RowCount}
	for i, row := range d.Rows {
		values := map[string]any{}
		for _, c := range columns {
			values[c.Name] = row[c.Name]
		}
		out.Rows[i] = values
	}
	return out, ValidateDataset(out)
}

func typedRow(row map[string]any, columns []Column) (map[string]any, error) {
	out := make(map[string]any, len(columns))
	for _, c := range columns {
		v := row[c.Name]
		if v == nil || (v == "" && c.Type != "string") {
			out[c.Name] = nil
			continue
		}
		s := fmt.Sprint(v)
		switch c.Type {
		case "string":
			out[c.Name] = s
		case "number":
			f, err := strconv.ParseFloat(s, 64)
			if !numberPattern.MatchString(s) || err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
				return nil, fmt.Errorf("column %q: %q is not a number", c.Name, short(s))
			}
			out[c.Name] = f
		case "boolean":
			if s != "true" && s != "false" {
				return nil, fmt.Errorf("column %q: use true or false", c.Name)
			}
			out[c.Name] = s == "true"
		}
	}
	return out, nil
}

func short(s string) string {
	if len(s) > 256 {
		return s[:256] + "..."
	}
	return s
}
