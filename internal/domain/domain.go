// Package domain defines stable audit identities, exact money and extraction contracts.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// ErrInvalid indicates a contract violation without exposing source content.
var ErrInvalid = errors.New("invalid audit data")

var identifier = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:@/-]{0,159}$`)
var moneyPattern = regexp.MustCompile(`^-?[0-9]{1,16}(\.[0-9]{1,4})?$`)
var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

// ValidID excludes delimiters used in stable identity construction.
func ValidID(s string) bool { return identifier.MatchString(s) }

// Hash returns a lowercase SHA-256 content address.
func Hash(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }

// ExceptionKey is independent of snapshot, rule version and execution time.
func ExceptionKey(rule, entity, id, period string) string {
	return Hash([]byte(strings.Join([]string{rule, entity, id, period}, "\x1f")))
}

// EntityID scopes natural keys to a source and orders duplicate pairs consistently.
func EntityID(source string, ids ...string) string {
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	return source + "|" + strings.Join(sorted, "|")
}

// Money parses numeric(20,4), rejecting exponent syntax, overflow and rounding.
func Money(s string) (decimal.Decimal, error) {
	if !moneyPattern.MatchString(s) {
		return decimal.Zero, ErrInvalid
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero, ErrInvalid
	}
	return d, nil
}

// Record is a canonical row for the initial payment, journal and approval tests.
// Monetary values are decimal strings in JSON and Parquet, never binary floats.
type Record struct {
	Entity            string `json:"entity" parquet:"entity"`
	ID                string `json:"id" parquet:"id"`
	Date              string `json:"date" parquet:"date"`
	Amount            string `json:"amount" parquet:"amount"`
	Currency          string `json:"currency" parquet:"currency"`
	ReportingAmount   string `json:"reporting_amount" parquet:"reporting_amount"`
	ReportingCurrency string `json:"reporting_currency" parquet:"reporting_currency"`
	ExchangeRate      string `json:"exchange_rate" parquet:"exchange_rate"`
	RateDate          string `json:"rate_date" parquet:"rate_date"`
	VendorID          string `json:"vendor_id" parquet:"vendor_id"`
	Debit             string `json:"debit" parquet:"debit"`
	Credit            string `json:"credit" parquet:"credit"`
	Limit             string `json:"limit" parquet:"limit"`
	Reversal          bool   `json:"reversal" parquet:"reversal"`
	Intercompany      bool   `json:"intercompany" parquet:"intercompany"`
}

// LogValue prevents accidental disclosure of financial fields.
func (r Record) LogValue() slog.Value { return slog.GroupValue(slog.String("entity_id", r.ID)) }

// Period is an explicit fiscal calendar range; its ID is not derived from dates.
type Period struct {
	ID    string `json:"id"`
	Start string `json:"start"`
	End   string `json:"end"`
}

// Validate checks period boundaries and natural keys.
func (p Period) Validate() error {
	a, e1 := time.Parse(time.DateOnly, p.Start)
	b, e2 := time.Parse(time.DateOnly, p.End)
	if !ValidID(p.ID) || e1 != nil || e2 != nil || b.Before(a) {
		return ErrInvalid
	}
	return nil
}

// Validate enforces the initial canonical extraction contract.
func (r Record) Validate(p Period) error {
	if !ValidID(r.ID) || !currencyPattern.MatchString(r.Currency) || !currencyPattern.MatchString(r.ReportingCurrency) {
		return ErrInvalid
	}
	d, e := time.Parse(time.DateOnly, r.Date)
	if e != nil {
		return ErrInvalid
	}
	a, _ := time.Parse(time.DateOnly, p.Start)
	b, _ := time.Parse(time.DateOnly, p.End)
	if d.Before(a) || d.After(b) {
		return ErrInvalid
	}
	if _, e = time.Parse(time.DateOnly, r.RateDate); e != nil {
		return ErrInvalid
	}
	for _, v := range []string{r.Amount, r.ReportingAmount, r.ExchangeRate, r.Debit, r.Credit, r.Limit} {
		if _, e = Money(v); e != nil {
			return e
		}
	}
	rate, _ := Money(r.ExchangeRate)
	if !rate.IsPositive() {
		return ErrInvalid
	}
	switch r.Entity {
	case "payment":
		amount, _ := Money(r.Amount)
		if !ValidID(r.VendorID) || !amount.IsPositive() {
			return ErrInvalid
		}
	case "journal_entry":
		debit, _ := Money(r.Debit)
		credit, _ := Money(r.Credit)
		if debit.IsNegative() || credit.IsNegative() || debit.IsPositive() == credit.IsPositive() {
			return ErrInvalid
		}
	case "approval":
		limit, _ := Money(r.Limit)
		if limit.IsNegative() {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

// Control is an independently supplied source control report, grouped by currency.
type Control struct {
	Entity   string `json:"entity"`
	Currency string `json:"currency"`
	Count    int    `json:"count"`
	Amount   string `json:"amount"`
	Debit    string `json:"debit"`
	Credit   string `json:"credit"`
}

// Population carries a complete source/period replacement, never an unmerged delta.
type Population struct {
	SourceID         string    `json:"source_id"`
	Period           Period    `json:"period"`
	ControlReference string    `json:"control_reference"`
	Controls         []Control `json:"controls"`
	Records          []Record  `json:"records"`
}

// Validate rejects ambiguous grains, duplicates and malformed independent controls.
func (p Population) Validate() error {
	if !ValidID(p.SourceID) || p.Period.Validate() != nil || len(p.ControlReference) < 1 || len(p.ControlReference) > 200 || len(p.Controls) == 0 || len(p.Records) > 100000 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, r := range p.Records {
		k := r.Entity + "|" + r.ID
		if seen[k] || r.Validate(p.Period) != nil {
			return ErrInvalid
		}
		seen[k] = true
	}
	seen = map[string]bool{}
	for _, c := range p.Controls {
		k := c.Entity + "|" + c.Currency
		if seen[k] || !currencyPattern.MatchString(c.Currency) || c.Count < 0 {
			return ErrInvalid
		}
		seen[k] = true
		if c.Entity != "payment" && c.Entity != "journal_entry" && c.Entity != "approval" {
			return ErrInvalid
		}
		for _, v := range []string{c.Amount, c.Debit, c.Credit} {
			if _, e := Money(v); e != nil {
				return e
			}
		}
	}
	return nil
}

// TieOutDifference retains expected/actual figures for authorized UI access only.
type TieOutDifference struct {
	Expected Control `json:"expected"`
	Actual   Control `json:"actual"`
}

// TieOut checks exact counts and transaction-currency totals before scoring.
func TieOut(p Population) []TieOutDifference {
	actual := map[string]Control{}
	expected := map[string]Control{}
	for _, c := range p.Controls {
		expected[c.Entity+"|"+c.Currency] = c
		actual[c.Entity+"|"+c.Currency] = Control{Entity: c.Entity, Currency: c.Currency, Amount: "0", Debit: "0", Credit: "0"}
	}
	for _, r := range p.Records {
		k := r.Entity + "|" + r.Currency
		c, ok := actual[k]
		if !ok {
			c = Control{Entity: r.Entity, Currency: r.Currency, Amount: "0", Debit: "0", Credit: "0"}
		}
		c.Count++
		a, _ := decimal.NewFromString(c.Amount)
		b, _ := Money(r.Amount)
		c.Amount = a.Add(b).StringFixed(4)
		a, _ = decimal.NewFromString(c.Debit)
		b, _ = Money(r.Debit)
		c.Debit = a.Add(b).StringFixed(4)
		a, _ = decimal.NewFromString(c.Credit)
		b, _ = Money(r.Credit)
		c.Credit = a.Add(b).StringFixed(4)
		actual[k] = c
	}
	var differences []TieOutDifference
	keys := make([]string, 0, len(actual))
	for k := range actual {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		a := actual[k]
		e, ok := expected[k]
		match := ok && a.Count == e.Count
		for _, pair := range [][2]string{{a.Amount, e.Amount}, {a.Debit, e.Debit}, {a.Credit, e.Credit}} {
			v, _ := decimal.NewFromString(pair[0])
			w, err := Money(pair[1])
			if err != nil || !v.Equal(w) {
				match = false
			}
		}
		if a.Entity == "journal_entry" {
			d, _ := decimal.NewFromString(a.Debit)
			c, _ := decimal.NewFromString(a.Credit)
			match = match && d.Equal(c)
		}
		if !match {
			differences = append(differences, TieOutDifference{Expected: e, Actual: a})
		}
	}
	return differences
}

// CanonicalJSON uses Go's sorted object keys for stable content hashes.
func CanonicalJSON(v any) ([]byte, error) { return json.Marshal(v) }
