// Package rules evaluates bounded, single-row JDM models with immutable versions.
package rules

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/Aswikinz/Autodit/internal/domain"
	zen "github.com/gorules/zen-go/v2"
)

// EngineVersion is persisted with every observation and must change with the binding.
const EngineVersion = "zen-go/2.0.1"

// Parameters are tenant inputs shared by all versions of the shipped models.
type Parameters struct {
	Materiality        string            `json:"materiality"`
	CurrencyThresholds map[string]string `json:"currency_thresholds,omitempty"`
	WeekendDays        []int             `json:"weekend_days"`
	ExceptionCeiling   int               `json:"exception_ceiling"`
}

// Defaults are demonstrative policy inputs; clients must approve their own parameters.
func Defaults() Parameters {
	return Parameters{Materiality: "1000.0000", WeekendDays: []int{0, 6}, ExceptionCeiling: 10000}
}

// Validate bounds configurable inputs without allowing arbitrary joins or code.
func (p Parameters) Validate() error {
	m, e := domain.Money(p.Materiality)
	if e != nil || m.IsNegative() || p.ExceptionCeiling < 1 || p.ExceptionCeiling > 100000 || len(p.WeekendDays) == 0 || len(p.WeekendDays) > 7 {
		return domain.ErrInvalid
	}
	for currency, threshold := range p.CurrencyThresholds {
		if len(currency) != 3 || currency[0] < 'A' || currency[0] > 'Z' || currency[1] < 'A' || currency[1] > 'Z' || currency[2] < 'A' || currency[2] > 'Z' {
			return domain.ErrInvalid
		}
		v, e := domain.Money(threshold)
		if e != nil || v.IsNegative() {
			return domain.ErrInvalid
		}
	}
	seen := map[int]bool{}
	for _, d := range p.WeekendDays {
		if d < 0 || d > 6 || seen[d] {
			return domain.ErrInvalid
		}
		seen[d] = true
	}
	return nil
}

// MaterialityFor requires an explicit transaction-currency policy. USD uses
// the primary threshold; other currencies must be configured individually.
func (p Parameters) MaterialityFor(currency string) (string, error) {
	if v, ok := p.CurrencyThresholds[currency]; ok {
		return v, nil
	}
	if currency == "USD" {
		return p.Materiality, nil
	}
	return "", domain.ErrInvalid
}

// Model is an immutable decision graph loaded from the shipped rulepack or database.
type Model struct {
	ID      string          `json:"rule_id"`
	Title   string          `json:"title"`
	Version string          `json:"version"`
	Content json.RawMessage `json:"model"`
}

// Result is the complete business decision for a single enriched candidate.
type Result struct {
	Flag      bool   `json:"flag"`
	Severity  string `json:"severity"`
	OwnerRole string `json:"owner_role"`
}

// Evaluation retains the exact input, result and engine trace.
type Evaluation struct {
	Result Result          `json:"result"`
	Trace  json.RawMessage `json:"trace"`
}

// ValidateModel permits only the shipped three-node decision-table topology.
// Function nodes, loaders, free-form expressions and graph cycles are rejected.
func ValidateModel(data []byte) error {
	if len(data) > 128*1024 {
		return domain.ErrInvalid
	}
	var graph struct {
		Nodes []struct {
			ID      string          `json:"id"`
			Type    string          `json:"type"`
			Content json.RawMessage `json:"content"`
		} `json:"nodes"`
		Edges []struct {
			SourceID string `json:"sourceId"`
			TargetID string `json:"targetId"`
		} `json:"edges"`
	}
	if json.Unmarshal(data, &graph) != nil || len(graph.Nodes) != 3 || len(graph.Edges) != 2 {
		return domain.ErrInvalid
	}
	types := map[string]string{}
	for _, n := range graph.Nodes {
		if types[n.ID] != "" {
			return domain.ErrInvalid
		}
		types[n.ID] = n.Type
		switch n.Type {
		case "inputNode", "outputNode":
		case "decisionTableNode":
			var c struct {
				HitPolicy string `json:"hitPolicy"`
				Inputs    []struct {
					ID    string `json:"id"`
					Field string `json:"field"`
				} `json:"inputs"`
				Outputs []struct {
					ID    string `json:"id"`
					Field string `json:"field"`
				} `json:"outputs"`
				Rules []map[string]string `json:"rules"`
			}
			if json.Unmarshal(n.Content, &c) != nil || c.HitPolicy != "first" || len(c.Inputs) != 1 || c.Inputs[0].Field != "qualifies" || len(c.Outputs) != 3 || len(c.Rules) != 2 {
				return domain.ErrInvalid
			}
			fields := map[string]string{}
			for _, out := range c.Outputs {
				if out.Field != "flag" && out.Field != "severity" && out.Field != "owner_role" {
					return domain.ErrInvalid
				}
				fields[out.Field] = out.ID
			}
			if len(fields) != 3 {
				return domain.ErrInvalid
			}
			for i, row := range c.Rules {
				condition := row[c.Inputs[0].ID]
				if (i == 0 && condition != "true") || (i == 1 && condition != "") {
					return domain.ErrInvalid
				}
				if row[fields["flag"]] != "true" && row[fields["flag"]] != "false" {
					return domain.ErrInvalid
				}
				sev := row[fields["severity"]]
				if sev != `"low"` && sev != `"medium"` && sev != `"high"` && sev != `"critical"` {
					return domain.ErrInvalid
				}
				owner := row[fields["owner_role"]]
				if owner != `"auditor"` && owner != `"audit_manager"` {
					return domain.ErrInvalid
				}
			}
		default:
			return domain.ErrInvalid
		}
	}
	links := map[string]bool{}
	for _, e := range graph.Edges {
		link := types[e.SourceID] + ":" + types[e.TargetID]
		links[link] = true
	}
	if !links["inputNode:decisionTableNode"] || !links["decisionTableNode:outputNode"] {
		return domain.ErrInvalid
	}
	return nil
}

// LoadPack reads the three shipped models; content hashes bind versions to bytes.
func LoadPack(dir string) ([]Model, error) {
	var result []Model
	for _, item := range []struct{ id, title, area string }{{"AP-01", "Duplicate payments", "ap"}, {"JE-01", "Weekend postings", "je"}, {"AP-02", "Approval limit breach", "ap"}} {
		b, e := os.ReadFile(filepath.Join(dir, item.area, item.id+".jdm.json"))
		if e != nil {
			return nil, errors.New("rulepack unavailable")
		}
		if e = ValidateModel(b); e != nil {
			return nil, e
		}
		var v any
		if json.Unmarshal(b, &v) != nil {
			return nil, domain.ErrInvalid
		}
		b, _ = json.Marshal(v)
		result = append(result, Model{ID: item.id, Title: item.title, Version: domain.Hash(b), Content: b})
	}
	return result, nil
}

// Evaluate uses a fresh engine, so no mutable graph state survives between calls.
func Evaluate(model []byte, input map[string]any) (Evaluation, error) {
	if err := ValidateModel(model); err != nil {
		return Evaluation{}, err
	}
	engine := zen.NewEngine(zen.EngineConfig{})
	defer engine.Dispose()
	decision, err := engine.CreateDecision(model)
	if err != nil {
		return Evaluation{}, errors.New("decision compilation failed")
	}
	defer decision.Dispose()
	response, err := decision.EvaluateWithOpts(input, zen.EvaluationOptions{Trace: true, MaxDepth: 8})
	if err != nil {
		return Evaluation{}, errors.New("decision evaluation failed")
	}
	var result Result
	if json.Unmarshal(response.Result, &result) != nil || response.Trace == nil {
		return Evaluation{}, errors.New("invalid decision output")
	}
	if result.Severity != "low" && result.Severity != "medium" && result.Severity != "high" && result.Severity != "critical" {
		return Evaluation{}, domain.ErrInvalid
	}
	if result.OwnerRole != "auditor" && result.OwnerRole != "audit_manager" {
		return Evaluation{}, domain.ErrInvalid
	}
	return Evaluation{Result: result, Trace: *response.Trace}, nil
}
