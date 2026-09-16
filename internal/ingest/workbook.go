package ingest

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"github.com/xuri/excelize/v2"
	"slices"
)

type Workbook struct {
	Sheets []string `json:"sheets"`
	Sheet  string   `json:"sheet"`
	CSV    string   `json:"csv"`
	Schema Schema   `json:"profile"`
}

// ReadWorkbook converts one explicitly selected worksheet into the same bounded CSV contract.
func ReadWorkbook(ctx context.Context, data []byte, sheet string) (Workbook, error) {
	var out Workbook
	if len(data) > 16*1024*1024 {
		return out, errors.New("workbook exceeds 16 MB")
	}
	f, e := excelize.OpenReader(bytes.NewReader(data), excelize.Options{UnzipSizeLimit: 64 * 1024 * 1024, UnzipXMLSizeLimit: 8 * 1024 * 1024})
	if e != nil {
		return out, errors.New("workbook is unreadable or exceeds extraction limits")
	}
	defer f.Close()
	out.Sheets = f.GetSheetList()
	if sheet == "" {
		return out, nil
	}
	if !slices.Contains(out.Sheets, sheet) {
		return out, errors.New("worksheet not found")
	}
	out.Sheet = sheet
	rows, e := f.Rows(sheet)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	var b bytes.Buffer
	writer := csv.NewWriter(&b)
	width := 0
	count := 0
	for rows.Next() {
		if e = ctx.Err(); e != nil {
			return out, e
		}
		cells, e := rows.Columns()
		if e != nil {
			return out, e
		}
		count++
		if count > 100001 || len(cells) > 200 {
			return out, errors.New("sheet exceeds 100000 records or 200 columns")
		}
		if count == 1 {
			width = len(cells)
		}
		if len(cells) > width {
			return out, errors.New("data extends past the header columns")
		}
		for len(cells) < width {
			cells = append(cells, "")
		}
		if e = writer.Write(cells); e != nil {
			return out, e
		}
		if b.Len() > 32*1024*1024 {
			return out, errors.New("converted sheet exceeds 32 MB")
		}
	}
	if e = rows.Error(); e != nil {
		return out, e
	}
	writer.Flush()
	if e = writer.Error(); e != nil {
		return out, e
	}
	out.CSV = b.String()
	out.Schema, e = (File{}).Discover(ctx, SourceConfig{CSV: out.CSV})
	return out, e
}
