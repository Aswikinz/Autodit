package ingest

import (
	"context"
	"github.com/xuri/excelize/v2"
	"os"
	"strings"
	"testing"
)

func TestWorkbookPreview(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	_ = f.SetSheetRow("Sheet1", "A1", &[]any{"id", "amount"})
	_ = f.SetSheetRow("Sheet1", "A2", &[]any{"payment-1", "1200.1234"})
	b, e := f.WriteToBuffer()
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	discovery, e := ReadWorkbook(ctx, b.Bytes(), "")
	if e != nil || len(discovery.Sheets) != 1 || discovery.CSV != "" {
		t.Fatal("sheet discovery failed", e)
	}
	out, e := ReadWorkbook(ctx, b.Bytes(), "Sheet1")
	if e != nil || out.Schema.Rows != 1 || out.Schema.Sample[0][1] != "1200.1234" {
		t.Fatal("exact value lost", e)
	}
	if _, e = ReadWorkbook(ctx, b.Bytes(), "missing"); e == nil {
		t.Fatal("missing sheet accepted")
	}
	if _, e = ReadWorkbook(ctx, []byte("invalid"), "Sheet1"); e == nil {
		t.Fatal("bad workbook accepted")
	}
}
func TestConnectionValidation(t *testing.T) {
	for _, kind := range []string{"postgres", "mysql", "sqlserver"} {
		c := Connection{Kind: kind, Host: "localhost", Port: 1234, Database: "audit", Username: "user", Password: "p@ss?;value"}
		db, e := c.open()
		if e != nil {
			t.Fatal(e)
		}
		db.Close()
		c.Host = "localhost/other"
		if _, e = c.open(); e == nil {
			t.Fatal("URL accepted as host")
		}
	}
	if quotedIdentifier("postgres", `a";drop table x;--`) != `"a"";drop table x;--"` {
		t.Fatal("identifier escaping failed")
	}
}
func TestDatabasePreviewIntegration(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("integration database required")
	}
	c := Connection{Kind: "postgres", Host: "autodit-test-postgres", Port: 5432, Database: "autodit", Username: "autodit_app", AllowPlaintext: true}
	tables, e := c.Tables(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, table := range tables {
		if table.Name == "schema_version" {
			p, e := c.Preview(context.Background(), table)
			if e != nil || len(p.Sample) == 0 {
				t.Fatal("table preview failed", e)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("table discovery empty")
	}
	if _, e = c.Preview(context.Background(), Table{Schema: "public", Name: "schema_version;drop table tenant"}); e == nil {
		t.Fatal("undiscovered table accepted")
	}
	for _, kind := range []string{"mysql", "sqlserver"} {
		if strings.Contains(tableQuery(kind), ";") {
			t.Fatal("unexpected multi statement discovery")
		}
	}
}
