package rules

import (
	"encoding/json"
	"testing"
)

func TestShippedModels(t *testing.T) {
	t.Parallel()
	models, e := LoadPack("../../rulepack")
	if e != nil {
		t.Fatal(e)
	}
	for _, m := range models {
		t.Run(m.ID, func(t *testing.T) {
			for _, v := range []any{true, false, nil} {
				out, e := Evaluate(m.Content, map[string]any{"qualifies": v})
				if e != nil {
					t.Fatal(e)
				}
				if out.Result.Flag != (v == true) || len(out.Trace) == 0 {
					t.Fatal("incorrect decision or missing trace")
				}
			}
			var graph map[string]any
			_ = json.Unmarshal(m.Content, &graph)
			nodes := graph["nodes"].([]any)
			nodes[1].(map[string]any)["type"] = "functionNode"
			bad, _ := json.Marshal(graph)
			if ValidateModel(bad) == nil {
				t.Fatal("executable node accepted")
			}
		})
	}
	for _, b := range [][]byte{nil, []byte(`{}`), []byte(`{"nodes":[]}`), make([]byte, 131073)} {
		if ValidateModel(b) == nil {
			t.Fatal("bad model accepted")
		}
		if _, e = Evaluate(b, nil); e == nil {
			t.Fatal("bad model evaluated")
		}
	}
	if _, e := LoadPack(t.TempDir()); e == nil {
		t.Fatal("missing pack accepted")
	}
}

func TestParameters(t *testing.T) {
	t.Parallel()
	if Defaults().Validate() != nil {
		t.Fatal("defaults invalid")
	}
	for _, edit := range []func(*Parameters){func(p *Parameters) { p.Materiality = "NaN" }, func(p *Parameters) { p.Materiality = "-1" }, func(p *Parameters) { p.ExceptionCeiling = 0 }, func(p *Parameters) { p.WeekendDays = nil }, func(p *Parameters) { p.WeekendDays = []int{8} }, func(p *Parameters) { p.WeekendDays = []int{0, 0} }} {
		p := Defaults()
		edit(&p)
		if p.Validate() == nil {
			t.Fatal("invalid parameters accepted")
		}
	}
}
