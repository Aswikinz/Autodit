// Package analytics generates candidates with explicit, engineering-owned populations.
package analytics

import (
	"context"
	_ "embed"
	"errors"
	"time"

	"github.com/Aswikinz/Autodit/internal/domain"
	"github.com/Aswikinz/Autodit/internal/rules"
	"github.com/jackc/pgx/v5"
)

//go:embed candidates.sql
var duplicatesSQL string

// Candidate carries a flat input and a stable, source-scoped entity identity.
type Candidate struct {
	RuleID   string
	Entity   string
	EntityID string
	Input    map[string]any
}

// ErrCeiling halts candidate explosions before materializing an unbounded queue.
var ErrCeiling = errors.New("candidate ceiling exceeded")

// RowCandidate enriches row-level tests with exact decimal qualification.
// ZEN decides flag/severity/routing; floating point never decides a money boundary.
func RowCandidate(source string, r domain.Record, p rules.Parameters) (Candidate, bool) {
	date, _ := time.Parse(time.DateOnly, r.Date)
	amount, _ := domain.Money(r.Amount)
	threshold, err := p.MaterialityFor(r.Currency)
	if err != nil {
		return Candidate{}, false
	}
	materiality, _ := domain.Money(threshold)
	c := Candidate{Entity: r.Entity, EntityID: domain.EntityID(source, r.ID), Input: map[string]any{"record_id": r.ID, "amount": r.Amount, "currency": r.Currency, "date": r.Date, "materiality": threshold}}
	switch r.Entity {
	case "journal_entry":
		c.RuleID = "JE-01"
		debit, _ := domain.Money(r.Debit)
		credit, _ := domain.Money(r.Credit)
		amount = debit.Add(credit)
		weekend := false
		for _, d := range p.WeekendDays {
			weekend = weekend || int(date.Weekday()) == d
		}
		c.Input["weekend"] = weekend
		c.Input["debit"] = r.Debit
		c.Input["credit"] = r.Credit
		c.Input["qualifies"] = weekend && amount.GreaterThanOrEqual(materiality)
	case "approval":
		c.RuleID = "AP-02"
		limit, _ := domain.Money(r.Limit)
		c.Input["approval_limit"] = r.Limit
		c.Input["qualifies"] = amount.GreaterThan(limit) && amount.GreaterThanOrEqual(materiality)
	default:
		return Candidate{}, false
	}
	return c, true
}

// Duplicates executes the blocked SQL over the current immutable canonical snapshot.
func Duplicates(ctx context.Context, tx pgx.Tx, tenant, snapshot, source string, p rules.Parameters) ([]Candidate, error) {
	rows, err := tx.Query(ctx, duplicatesSQL, tenant, snapshot, p.ExceptionCeiling+1)
	if err != nil {
		return nil, errors.New("candidate query failed")
	}
	defer rows.Close()
	out := []Candidate{}
	for rows.Next() {
		var a, b, amount, currency, dateA, dateB string
		if err = rows.Scan(&a, &b, &amount, &currency, &dateA, &dateB); err != nil {
			return nil, errors.New("candidate decode failed")
		}
		m, err := domain.Money(amount)
		if err != nil {
			return nil, err
		}
		threshold, err := p.MaterialityFor(currency)
		if err != nil {
			return nil, err
		}
		materiality, _ := domain.Money(threshold)
		out = append(out, Candidate{RuleID: "AP-01", Entity: "payment", EntityID: domain.EntityID(source, a, b), Input: map[string]any{"record_id": a, "paired_record_id": b, "amount": amount, "currency": currency, "date": dateA, "paired_date": dateB, "materiality": threshold, "qualifies": m.GreaterThanOrEqual(materiality)}})
		if len(out) > p.ExceptionCeiling {
			return nil, ErrCeiling
		}
	}
	if rows.Err() != nil {
		return nil, errors.New("candidate read failed")
	}
	return out, nil
}
