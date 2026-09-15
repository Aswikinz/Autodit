package ingest

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"github.com/Aswikinz/Autodit/internal/domain"
	"os"
	"strings"
	"testing"
)

func TestCSVContractAndDiscovery(t *testing.T) {
	t.Parallel()
	b, e := os.ReadFile("../../test/fixtures/population.json")
	if e != nil {
		t.Fatal(e)
	}
	var p domain.Population
	_ = json.Unmarshal(b, &p)
	header := []string{"entity", "id", "date", "amount", "currency", "reporting_amount", "reporting_currency", "exchange_rate", "rate_date", "vendor_id", "debit", "credit", "limit", "reversal", "intercompany"}
	var buf strings.Builder
	w := csv.NewWriter(&buf)
	_ = w.Write(header)
	for _, r := range p.Records {
		_ = w.Write([]string{r.Entity, r.ID, r.Date, r.Amount, r.Currency, r.ReportingAmount, r.ReportingCurrency, r.ExchangeRate, r.RateDate, r.VendorID, r.Debit, r.Credit, r.Limit, "false", "false"})
	}
	w.Flush()
	p.Records = nil
	c := SourceConfig{CSV: buf.String(), Population: p}
	f := File{}
	s, e := f.Discover(context.Background(), c)
	if e != nil || s.Rows != 5 || s.Nulls["vendor_id"] != 3 {
		t.Fatal("incorrect profile", s, e)
	}
	if e = f.TestConnection(context.Background(), c); e != nil {
		t.Fatal(e)
	}
	out, e := f.Extract(context.Background(), c, Watermark{})
	if e != nil || len(out.Population.Records) != 5 || len(out.Watermark.Value) != 64 {
		t.Fatal("extract failed", e)
	}
	if len(domain.TieOut(out.Population)) != 0 {
		t.Fatal("CSV lost exact values")
	}
	for _, bad := range []string{"", "a,a\n1,2\n", "a,b\n1\n", strings.Replace(c.CSV, "false", "maybe", 1), strings.Replace(c.CSV, "amount,", "missing,", 1)} {
		if _, e = f.Extract(context.Background(), SourceConfig{CSV: bad, Population: p}, Watermark{}); e == nil {
			t.Fatal("malformed CSV accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = f.Discover(ctx, c); e == nil {
		t.Fatal("ignored cancellation")
	}
	c.Mapping = map[string]string{"unknown": "id"}
	if _, e = f.Extract(context.Background(), c, Watermark{}); e == nil {
		t.Fatal("unknown canonical mapping accepted")
	}
}
