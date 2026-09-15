package analytics

import (
	"github.com/Aswikinz/Autodit/internal/domain"
	"github.com/Aswikinz/Autodit/internal/rules"
	"strings"
	"testing"
)

func TestExactRowBoundaries(t *testing.T) {
	t.Parallel()
	p := rules.Defaults()
	for _, tc := range []struct {
		entity, date, amount, debit, limit string
		want                               bool
	}{{"journal_entry", "2026-08-01", "0", "1000", "0", true}, {"journal_entry", "2026-08-03", "0", "1000", "0", false}, {"journal_entry", "2026-08-01", "0", "999.9999", "0", false}, {"approval", "2026-08-01", "1000", "0", "999.9999", true}, {"approval", "2026-08-01", "1000", "0", "1000", false}, {"approval", "2026-08-01", "999.9999", "0", "0", false}} {
		c, ok := RowCandidate("s", domain.Record{Entity: tc.entity, ID: "1", Date: tc.date, Amount: tc.amount, Debit: tc.debit, Credit: "0", Limit: tc.limit, Currency: "USD"}, p)
		if !ok || c.Input["qualifies"] != tc.want {
			t.Errorf("boundary %+v failed", tc)
		}
	}
	if _, ok := RowCandidate("s", domain.Record{Entity: "payment"}, p); ok {
		t.Fatal("payment evaluated without set-based pairing")
	}
}

func TestBlockingKeys(t *testing.T) {
	t.Parallel()
	for _, constraint := range []string{"a.vendor_id=b.vendor_id", "a.currency_code=b.currency_code", "a.amount=b.amount", "a.tenant_id=$1", "a.snapshot_id=$2", "a.source_record_id<b.source_record_id", "between a.posting_date-30 and a.posting_date+30", "not a.reversal", "not b.intercompany", "limit $3"} {
		if !strings.Contains(duplicatesSQL, constraint) {
			t.Errorf("missing population constraint %s", constraint)
		}
	}
}
