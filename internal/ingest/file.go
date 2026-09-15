// Package ingest defines the source connector seam and a strict CSV implementation.
package ingest

import (
	"context"
	"encoding/csv"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/Aswikinz/Autodit/internal/domain"
)

// SourceConfig supplies complete file bytes and optional canonical-to-source mappings.
type SourceConfig struct {
	CSV        string
	Mapping    map[string]string
	Population domain.Population
}

// Schema is a discovered header and bounded profile.
type Schema struct {
	Columns []string       `json:"columns"`
	Rows    int            `json:"rows"`
	Nulls   map[string]int `json:"nulls"`
}

// Watermark identifies the last committed extract, never an in-progress one.
type Watermark struct{ Value string }

// ExtractResult is a full population and the proposed next watermark.
type ExtractResult struct {
	Population domain.Population
	Watermark  Watermark
}

// SourceConnector isolates source-specific discovery and extraction.
type SourceConnector interface {
	Discover(context.Context, SourceConfig) (Schema, error)
	Extract(context.Context, SourceConfig, Watermark) (ExtractResult, error)
	TestConnection(context.Context, SourceConfig) error
}

// File reads a complete CSV extract with controls supplied independently.
type File struct{}

func rows(ctx context.Context, c SourceConfig) ([][]string, error) {
	if len(c.CSV) > 32*1024*1024 {
		return nil, domain.ErrInvalid
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(c.CSV, "\ufeff")))
	out := [][]string{}
	for {
		row, e := r.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, errors.New("invalid CSV shape")
		}
		out = append(out, row)
		if len(out) > 100001 {
			return nil, errors.New("CSV exceeds population limit")
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	if len(out) == 0 {
		return nil, errors.New("CSV header required")
	}
	seen := map[string]bool{}
	for _, h := range out[0] {
		if h == "" || seen[h] {
			return nil, errors.New("CSV headers must be nonempty and unique")
		}
		seen[h] = true
	}
	return out, nil
}

// Discover profiles all rows, so the count is a completeness diagnostic, not a control figure.
func (File) Discover(ctx context.Context, c SourceConfig) (Schema, error) {
	rs, e := rows(ctx, c)
	if e != nil {
		return Schema{}, e
	}
	s := Schema{Columns: rs[0], Rows: len(rs) - 1, Nulls: map[string]int{}}
	for _, r := range rs[1:] {
		for i, v := range r {
			if v == "" {
				s.Nulls[rs[0][i]]++
			}
		}
	}
	return s, nil
}

// TestConnection verifies the extract is readable without mutating source state.
func (f File) TestConnection(ctx context.Context, c SourceConfig) error {
	_, e := f.Discover(ctx, c)
	return e
}

// Extract maps only known canonical columns, then validates every row and its grain.
func (File) Extract(ctx context.Context, c SourceConfig, _ Watermark) (ExtractResult, error) {
	rs, e := rows(ctx, c)
	if e != nil {
		return ExtractResult{}, e
	}
	headers := map[string]int{}
	for i, h := range rs[0] {
		headers[h] = i
	}
	fields := []string{"entity", "id", "date", "amount", "currency", "reporting_amount", "reporting_currency", "exchange_rate", "rate_date", "vendor_id", "debit", "credit", "limit", "reversal", "intercompany"}
	indices := map[string]int{}
	for _, field := range fields {
		header := field
		if c.Mapping[field] != "" {
			header = c.Mapping[field]
		}
		idx, ok := headers[header]
		if !ok {
			return ExtractResult{}, errors.New("required canonical column missing")
		}
		indices[field] = idx
	}
	for field := range c.Mapping {
		if _, ok := indices[field]; !ok {
			return ExtractResult{}, domain.ErrInvalid
		}
	}
	p := c.Population
	p.Records = []domain.Record{}
	for _, row := range rs[1:] {
		v := func(field string) string { return row[indices[field]] }
		reversal, e1 := strconv.ParseBool(v("reversal"))
		intercompany, e2 := strconv.ParseBool(v("intercompany"))
		if e1 != nil || e2 != nil {
			return ExtractResult{}, domain.ErrInvalid
		}
		p.Records = append(p.Records, domain.Record{Entity: v("entity"), ID: v("id"), Date: v("date"), Amount: v("amount"), Currency: v("currency"), ReportingAmount: v("reporting_amount"), ReportingCurrency: v("reporting_currency"), ExchangeRate: v("exchange_rate"), RateDate: v("rate_date"), VendorID: v("vendor_id"), Debit: v("debit"), Credit: v("credit"), Limit: v("limit"), Reversal: reversal, Intercompany: intercompany})
	}
	if e = p.Validate(); e != nil {
		return ExtractResult{}, e
	}
	return ExtractResult{Population: p, Watermark: Watermark{Value: domain.Hash([]byte(c.CSV))}}, nil
}
