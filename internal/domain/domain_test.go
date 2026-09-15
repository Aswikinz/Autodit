package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func fixture() Population {
	return Population{SourceID: "source-1", Period: Period{ID: "FY26-P08", Start: "2026-08-01", End: "2026-08-31"}, ControlReference: "independent-report-1", Controls: []Control{{Entity: "payment", Currency: "USD", Count: 1, Amount: "0.1000", Debit: "0", Credit: "0"}}, Records: []Record{{Entity: "payment", ID: "P1", Date: "2026-08-01", Amount: "0.1000", Currency: "USD", ReportingAmount: "0.1000", ReportingCurrency: "USD", ExchangeRate: "1", RateDate: "2026-08-01", VendorID: "V1", Debit: "0", Credit: "0", Limit: "0"}}}
}

func TestIdentity(t *testing.T) {
	t.Parallel()
	if got := ExceptionKey("AP-01", "payment", "source|A|B", "FY26-P08"); got != "58050864010b06dc1722602eae76ca915518e2a504d6336524b0971784e3c891" {
		t.Fatalf("fixture key changed: %s", got)
	}
	if EntityID("s", "B", "A") != EntityID("s", "A", "B") {
		t.Fatal("pair identity depends on order")
	}
	if EntityID("a", "1") == EntityID("b", "1") {
		t.Fatal("sources collide")
	}
	if ValidID("a\x1fb") || ValidID("a|b") || ValidID("") {
		t.Fatal("ambiguous key accepted")
	}
	for _, part := range []string{"rule", "entity", "id", "period"} {
		args := []string{"r", "e", "i", "p"}
		switch part {
		case "rule":
			args[0] += "2"
		case "entity":
			args[1] += "2"
		case "id":
			args[2] += "2"
		case "period":
			args[3] += "2"
		}
		if ExceptionKey(args[0], args[1], args[2], args[3]) == ExceptionKey("r", "e", "i", "p") {
			t.Fatal("identity collision")
		}
	}
}

func TestMoney(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"1e5", "NaN", "0.00001", "10000000000000000", "", "+1"} {
		if _, err := Money(s); err == nil {
			t.Errorf("accepted invalid numeric %q", s)
		}
	}
	a, _ := Money("0.1")
	b, _ := Money("0.2")
	if a.Add(b).String() != "0.3" {
		t.Fatal("lost decimal precision")
	}
}

func TestPopulationAndTieOut(t *testing.T) {
	t.Parallel()
	p := fixture()
	if p.Validate() != nil || len(TieOut(p)) != 0 {
		t.Fatal("valid population rejected")
	}
	for _, tc := range []struct {
		name string
		edit func(*Population)
	}{
		{"duplicate grain", func(p *Population) { p.Records = append(p.Records, p.Records[0]) }},
		{"invalid source", func(p *Population) { p.SourceID = "" }},
		{"missing authority", func(p *Population) { p.ControlReference = "" }},
		{"bad calendar", func(p *Population) { p.Period.End = "2026-07-01" }},
		{"calendar parse", func(p *Population) { p.Period.Start = "today" }},
		{"period key", func(p *Population) { p.Period.ID = "" }},
		{"outside period", func(p *Population) { p.Records[0].Date = "2026-09-01" }},
		{"bad date", func(p *Population) { p.Records[0].Date = "bad" }},
		{"bad rate date", func(p *Population) { p.Records[0].RateDate = "bad" }},
		{"bad currency", func(p *Population) { p.Records[0].Currency = "usd" }},
		{"bad money", func(p *Population) { p.Records[0].Amount = "bad" }},
		{"zero rate", func(p *Population) { p.Records[0].ExchangeRate = "0" }},
		{"negative payment", func(p *Population) { p.Records[0].Amount = "-1" }},
		{"bad vendor", func(p *Population) { p.Records[0].VendorID = "" }},
		{"unknown entity", func(p *Population) { p.Records[0].Entity = "unknown" }},
		{"journal both sides", func(p *Population) { p.Records[0].Entity = "journal_entry" }},
		{"approval negative limit", func(p *Population) { p.Records[0].Entity = "approval"; p.Records[0].Limit = "-1" }},
		{"duplicate control", func(p *Population) { p.Controls = append(p.Controls, p.Controls[0]) }},
		{"control entity", func(p *Population) { p.Controls[0].Entity = "x" }},
		{"control amount", func(p *Population) { p.Controls[0].Amount = "NaN" }},
		{"control count", func(p *Population) { p.Controls[0].Count = -1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := fixture()
			tc.edit(&q)
			if q.Validate() == nil {
				t.Fatal("invalid population accepted")
			}
		})
	}
	for _, tc := range []struct {
		name string
		edit func(*Population)
	}{
		{"count mismatch", func(p *Population) { p.Controls[0].Count = 2 }},
		{"amount mismatch", func(p *Population) { p.Controls[0].Amount = "0.1001" }},
		{"fanout", func(p *Population) { p.Records = append(p.Records, p.Records[0], p.Records[0]) }},
		{"missing currency control", func(p *Population) { p.Records[0].Currency = "EUR" }},
		{"unbalanced journal", func(p *Population) {
			p.Records[0].Entity = "journal_entry"
			p.Records[0].Debit = "1"
			p.Controls[0].Entity = "journal_entry"
			p.Controls[0].Debit = "1"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := fixture()
			tc.edit(&q)
			if len(TieOut(q)) == 0 {
				t.Fatal("incomplete population passed")
			}
		})
	}
	p.Records[0].Entity = "approval"
	if p.Records[0].Validate(p.Period) != nil {
		t.Fatal("valid approval rejected")
	}
	p.Records[0].Entity = "journal_entry"
	p.Records[0].Debit = "1"
	if p.Records[0].Validate(p.Period) != nil {
		t.Fatal("valid journal rejected")
	}
	b, e := CanonicalJSON(map[string]string{"b": "2", "a": "1"})
	if e != nil || string(b) != `{"a":"1","b":"2"}` {
		t.Fatal("unstable serialization")
	}
	logJSON, _ := json.Marshal(p.Records[0].LogValue().String())
	if strings.Contains(string(logJSON), "amount") {
		t.Fatal("unsafe log representation")
	}
}
