package ingest

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCompleteDatabaseReadIntegration(t *testing.T) {
	value := os.Getenv("TEST_OWNER_DATABASE_URL")
	if value == "" {
		t.Skip("integration database required")
	}
	u, e := url.Parse(value)
	if e != nil {
		t.Fatal(e)
	}
	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		port = 5432
	}
	password, _ := u.User.Password()
	c := Connection{Kind: "postgres", Host: u.Hostname(), Port: port, Database: strings.TrimPrefix(u.Path, "/"), Username: u.User.Username(), Password: password, AllowPlaintext: true}
	db, e := c.open()
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		rows int
		cell int
		fail bool
	}{{"complete", 51, 5000, false}, {"too_many_rows", 10001, 1, true}, {"too_many_bytes", 4000, 5000, true}} {
		t.Run(tc.name, func(t *testing.T) {
			name := fmt.Sprintf("analysis_read_%d", time.Now().UnixNano())
			identifier := quotedIdentifier("postgres", name)
			_, e := db.ExecContext(ctx, "create view "+identifier+" as select n as id,repeat('x',"+strconv.Itoa(tc.cell)+") as text_value from generate_series(1,"+strconv.Itoa(tc.rows)+") n")
			if e != nil {
				t.Fatal(e)
			}
			defer func() { _, _ = db.ExecContext(ctx, "drop view "+identifier) }()
			out, e := c.Read(ctx, Table{Schema: "public", Name: name})
			if tc.fail {
				if e == nil {
					t.Fatal("oversized table accepted")
				}
				return
			}
			if e != nil || out.Truncated || len(out.Sample) != 51 || len(out.Sample[0][1]) != 5000 {
				t.Fatalf("complete data was truncated: %d rows, %v", len(out.Sample), e)
			}
			preview, e := c.Preview(ctx, Table{Schema: "public", Name: name})
			if e != nil || !preview.Truncated || len(preview.Sample) != 50 || len(preview.Sample[0][1]) != 4099 {
				t.Fatal("bounded preview changed", e)
			}
		})
	}
	if _, e = c.Read(ctx, Table{Schema: "public", Name: "missing_table"}); e == nil {
		t.Fatal("undiscovered table accepted")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = c.Read(cancelled, Table{Schema: "public", Name: "schema_version"}); e == nil {
		t.Fatal("cancelled read accepted")
	}
}
