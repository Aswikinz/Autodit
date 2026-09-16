package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if WorkerMain() {
		return
	}
	os.Exit(m.Run())
}

func graph(kind string, content any) []byte {
	nodes := []map[string]any{{"id": "in", "name": "Request", "type": "inputNode", "position": map[string]int{"x": 0, "y": 0}}}
	edges := []map[string]any{}
	if kind != "" {
		nodes = append(nodes, map[string]any{"id": "rule", "name": "Rule", "type": kind, "content": content, "position": map[string]int{"x": 250, "y": 0}})
		edges = append(edges, map[string]any{"id": "a", "sourceId": "in", "targetId": "rule", "type": "edge"}, map[string]any{"id": "b", "sourceId": "rule", "targetId": "out", "type": "edge"})
	} else {
		edges = append(edges, map[string]any{"id": "a", "sourceId": "in", "targetId": "out", "type": "edge"})
	}
	nodes = append(nodes, map[string]any{"id": "out", "name": "Response", "type": "outputNode", "position": map[string]int{"x": 500, "y": 0}})
	b, _ := json.Marshal(map[string]any{"nodes": nodes, "edges": edges})
	return b
}
func sample(t *testing.T) Dataset {
	t.Helper()
	d, e := ParseCSV(context.Background(), "ID,Amount,Active\n001,125,true\n002,5,false\n")
	if e != nil {
		t.Fatal(e)
	}
	return d
}

func TestEvaluateNodeTypes(t *testing.T) {
	d := sample(t)
	for _, tc := range []struct {
		name, kind string
		content    any
		flagged    int
	}{
		{"passthrough", "", nil, 0},
		{"table", "decisionTableNode", map[string]any{"hitPolicy": "first", "inputs": []any{map[string]string{"id": "i", "name": "Amount", "field": "data[\"Amount\"]"}}, "outputs": []any{map[string]string{"id": "o", "name": "Flag", "field": "flag"}}, "rules": []any{map[string]string{"_id": "r1", "i": "> 100", "o": "true"}, map[string]string{"_id": "r2", "i": "", "o": "false"}}}, 1},
		{"expression", "expressionNode", map[string]any{"expressions": []any{map[string]string{"id": "e", "key": "flag", "value": "data.Amount > 100"}}}, 1},
		{"function legacy", "functionNode", "const handler = input => ({flag:input.data.Amount > 100});", 1},
		{"function module", "functionNode", map[string]string{"source": "export const handler = async input => ({flag:input.data.Amount > 100});"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report, err := evaluateDirect(context.Background(), graph(tc.kind, tc.content), d)
			if err != nil {
				t.Fatal(err)
			}
			if report.Errors != 0 || report.Flagged != tc.flagged || len(report.Results) != 2 {
				t.Fatalf("unexpected report: %+v", report)
			}
			if len(report.Results[0].Trace) == 0 {
				t.Fatal("missing trace")
			}
		})
	}
}

func TestParsingAndSelection(t *testing.T) {
	d := sample(t)
	if d.Columns[0].Type != "string" || d.Columns[1].Type != "number" || d.Columns[2].Type != "boolean" {
		t.Fatal(d.Columns)
	}
	s, err := SelectColumns(d, []Column{{Name: "Amount", Type: "number"}})
	if err != nil || len(s.Rows[0]) != 1 || s.RowCount != 2 {
		t.Fatal(s, err)
	}
	j, err := ParseJSON(context.Background(), []byte(`[{"amount":12345678901234567890.12,"id":"001","active":true},{"amount":5,"active":false}]`))
	if err != nil || j.Rows[0]["amount"] != "12345678901234567890.12" || j.Rows[1]["id"] != nil || j.Columns[1].Type != "string" {
		t.Fatal(j, err)
	}
	for _, csv := range []string{"", "x,x\n1,2", "x,y\n1", "x\n", "\n", "x\n1\n2,3"} {
		if _, err := ParseCSV(context.Background(), csv); err == nil {
			t.Fatalf("accepted %q", csv)
		}
	}
	for _, raw := range []string{`[]`, `[null]`, `[1]`, `[{"x":{}}]`, `[{"x":[]}]`, `[{"x":1}] {}`, `[{"":1}]`} {
		if _, err := ParseJSON(context.Background(), []byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ParseCSV(ctx, "x\n1"); err == nil {
		t.Fatal("cancel ignored")
	}
	if _, err := ParseJSON(ctx, []byte(`[{"x":1}]`)); err == nil {
		t.Fatal("cancel ignored")
	}
	if _, err := FromRows(context.Background(), []string{"x"}, [][]string{{"1", "2"}}); err == nil {
		t.Fatal("invalid shape")
	}
	if _, err := SelectColumns(d, []Column{{Name: "missing", Type: "number"}}); err == nil {
		t.Fatal("unknown selected column")
	}
	if _, err := SelectColumns(d, []Column{{Name: "Amount", Type: "money"}}); err == nil {
		t.Fatal("unknown type")
	}
	if _, err := SelectColumns(d, []Column{{Name: "Amount", Type: "number"}, {Name: "Amount", Type: "number"}}); err == nil {
		t.Fatal("duplicate selected column")
	}
}

func TestRowErrorsAndCancellation(t *testing.T) {
	d := sample(t)
	d.Rows[0]["Amount"] = "not a number"
	r, e := Evaluate(context.Background(), graph("", nil), d)
	if e != nil || r.Errors != 1 || len(r.Results) != 2 || r.Results[0].Error == "" || r.Results[1].Error != "" {
		t.Fatal(r, e)
	}
	d.Rows[0]["Amount"] = "125"
	d.Rows[0]["Active"] = "yes"
	r, e = Evaluate(context.Background(), graph("", nil), d)
	if e != nil || r.Errors != 1 {
		t.Fatal(r, e)
	}
	r, e = Evaluate(context.Background(), graph("functionNode", "const handler = () => { throw Error('bad row') };"), sample(t))
	if e != nil || r.Errors != 2 || !strings.Contains(r.Results[0].Error, "bad row") {
		t.Fatal(r, e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = Evaluate(ctx, graph("", nil), sample(t)); e == nil {
		t.Fatal("cancel ignored")
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if _, e = Evaluate(ctx, graph("functionNode", "const handler = () => { while (true) {} };"), sample(t)); e == nil {
		t.Fatal("deadline ignored")
	}
}

func tableContent(field, policy string) map[string]any {
	return map[string]any{"hitPolicy": policy, "inputs": []any{map[string]string{"id": "i", "name": "Amount", "field": field}}, "outputs": []any{map[string]string{"id": "o", "name": "Flag", "field": "flag"}}, "rules": []any{map[string]string{"_id": "r1", "i": "> 100", "o": "true"}, map[string]string{"_id": "r2", "i": "", "o": "false"}}}
}

func TestCollectedFlagsAndArbitraryHeaders(t *testing.T) {
	r, err := Evaluate(context.Background(), graph("decisionTableNode", tableContent(`data["Amount"]`, "collect")), sample(t))
	if err != nil || r.Errors != 0 || r.Flagged != 1 {
		t.Fatal(r, err)
	}
	for _, header := range []string{"Net amount", "Invoice.Amount", `A["B"]`, `O'Brien "quoted"`, `A\B`, `__proto__`, `constructor`, "الكمية"} {
		d, err := FromRows(context.Background(), []string{header}, [][]string{{"150"}, {"5"}})
		if err != nil {
			t.Fatal(err)
		}
		key := zenString(header)
		r, err := Evaluate(context.Background(), graph("decisionTableNode", tableContent("data[("+key+")]", "first")), d)
		if err != nil || r.Errors != 0 || r.Flagged != 1 {
			t.Fatalf("header %s: %+v %v", header, r, err)
		}
	}
	var model map[string]any
	_ = json.Unmarshal(graph("", nil), &model)
	model["_autodit"] = map[string]any{"mode": "simple", "conditions": []any{}}
	b, _ := json.Marshal(model)
	if _, err := Evaluate(context.Background(), b, sample(t)); err != nil {
		t.Fatal(err)
	}
}

func TestEmptyAndNumericConditions(t *testing.T) {
	d, e := ParseCSV(context.Background(), "Amount\n125\n\"\"\n5\n")
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		condition string
		want      int
	}{{`null, ""`, 1}, {`$ != null and $ != ""`, 2}, {"> 100", 1}} {
		content := tableContent(`data.Amount`, "first")
		content["rules"].([]any)[0].(map[string]string)["i"] = tc.condition
		r, e := evaluateDirect(context.Background(), graph("decisionTableNode", content), d)
		if e != nil || r.Errors != 0 || r.Flagged != tc.want {
			t.Fatal(tc, r, e)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := evaluateDirect(ctx, graph("", nil), d); e == nil {
		t.Fatal("direct cancellation ignored")
	}
	if _, e := evaluateDirect(ctx, []byte(`{}`), d); e == nil {
		t.Fatal("invalid graph")
	}
	d.RowCount = 0
	if _, e := evaluateDirect(ctx, graph("", nil), d); e == nil {
		t.Fatal("invalid dataset")
	}
}

func zenString(s string) string {
	parts := strings.Split(s, `"`)
	for i := range parts {
		parts[i] = `"` + parts[i] + `"`
	}
	return strings.Join(parts, ` + '"' + `)
}

func TestConditionStringLiterals(t *testing.T) {
	for _, value := range []string{`invoice "quoted"`, `O'Brien "both"`, `A\B`, "with\ttab", "two\nlines"} {
		d, e := FromRows(context.Background(), []string{"Text"}, [][]string{{value}, {"different"}})
		if e != nil {
			t.Fatal(e)
		}
		content := tableContent("data.Text", "first")
		content["rules"].([]any)[0].(map[string]string)["i"] = "(" + zenString(value) + ")"
		r, e := evaluateDirect(context.Background(), graph("decisionTableNode", content), d)
		if e != nil || r.Errors != 0 || r.Flagged != 1 {
			t.Fatalf("literal %q: %+v %v", value, r, e)
		}
	}
}

func TestSyntaxPreflightRejectsFalseNegatives(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"condition", func(c map[string]any) { c["rules"].([]any)[0].(map[string]string)["i"] = "> >" }},
		{"field", func(c map[string]any) { c["inputs"].([]any)[0].(map[string]string)["field"] = `data["bad"` }},
		{"empty field", func(c map[string]any) { c["inputs"].([]any)[0].(map[string]string)["field"] = "" }},
		{"output", func(c map[string]any) { c["rules"].([]any)[0].(map[string]string)["o"] = `"unterminated` }},
		{"input transform", func(c map[string]any) { c["inputField"] = "data[" }},
		{"input default", func(c map[string]any) { c["inputs"].([]any)[0].(map[string]string)["defaultValue"] = "[" }},
		{"output default", func(c map[string]any) { c["outputs"].([]any)[0].(map[string]string)["defaultValue"] = "[" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			content := tableContent("data.Amount", "first")
			tc.change(content)
			model := graph("decisionTableNode", content)
			if err := validateSyntax(model); err == nil {
				t.Fatal("invalid expression accepted for saving")
			}
			if r, err := Evaluate(context.Background(), model, sample(t)); err == nil {
				t.Fatalf("invalid expression reported as successful: %+v", r)
			}
		})
	}
	broken := tableContent("data.Amount", "first")
	broken["rules"].([]any)[0].(map[string]string)["i"] = "> >"
	if err := ValidateSyntax(context.Background(), graph("decisionTableNode", broken)); err == nil {
		t.Fatal("save worker accepted malformed rule")
	}
	for _, model := range [][]byte{
		graph("expressionNode", map[string]any{"expressions": []any{map[string]string{"id": "x", "key": "flag", "value": "data["}}}),
		graph("expressionNode", map[string]any{"inputField": "data[", "expressions": []any{map[string]string{"id": "x", "key": "flag", "value": "true"}}}),
	} {
		if err := ValidateSyntax(context.Background(), model); err == nil {
			t.Fatal("malformed expression accepted")
		}
	}
	var switchModel map[string]any
	_ = json.Unmarshal(switchGraph(), &switchModel)
	switchModel["nodes"].([]any)[1].(map[string]any)["content"].(map[string]any)["statements"].([]any)[0].(map[string]any)["condition"] = "data["
	b, _ := json.Marshal(switchModel)
	if err := ValidateSyntax(context.Background(), b); err == nil {
		t.Fatal("malformed switch accepted")
	}
	// Fields that are produced by preceding nodes cannot be evaluated against an
	// empty sample. Their valid syntax remains accepted until the actual row runs.
	valid := graph("decisionTableNode", tableContent("data.Amount + 1", "first"))
	if err := ValidateSyntax(context.Background(), valid); err != nil {
		t.Fatal("context-dependent expression rejected", err)
	}
	if err := ValidateSyntax(context.Background(), []byte(`{}`)); err == nil {
		t.Fatal("invalid graph accepted")
	}
	full := tableContent("data.Amount", "first")
	full["inputs"] = []any{map[string]any{"id": "i", "field": nil}}
	full["rules"].([]any)[0].(map[string]string)["i"] = "data.Amount > 100"
	r, err := evaluateDirect(context.Background(), graph("decisionTableNode", full), sample(t))
	if err != nil || r.Flagged != 1 {
		t.Fatal("full expression columns failed", r, err)
	}
}

func switchGraph() []byte {
	var model map[string]any
	_ = json.Unmarshal(graph("switchNode", map[string]any{"hitPolicy": "first", "statements": []any{map[string]any{"id": "high", "condition": "data.Amount > 100", "isDefault": false}, map[string]any{"id": "low", "condition": "", "isDefault": true}}}), &model)
	nodes := model["nodes"].([]any)
	for i, flag := range []bool{true, false} {
		nodes = append(nodes, map[string]any{"id": fmt.Sprint("branch", i), "type": "expressionNode", "name": fmt.Sprint("Branch ", i), "position": map[string]int{"x": 400, "y": i * 100}, "content": map[string]any{"expressions": []any{map[string]string{"id": "flag", "key": "flag", "value": fmt.Sprint(flag)}}}})
	}
	model["nodes"] = nodes
	model["edges"] = []any{map[string]string{"id": "a", "sourceId": "in", "targetId": "rule", "type": "edge"}, map[string]string{"id": "b", "sourceId": "rule", "sourceHandle": "high", "targetId": "branch0", "type": "edge"}, map[string]string{"id": "c", "sourceId": "rule", "sourceHandle": "low", "targetId": "branch1", "type": "edge"}, map[string]string{"id": "d", "sourceId": "branch0", "targetId": "out", "type": "edge"}, map[string]string{"id": "e", "sourceId": "branch1", "targetId": "out", "type": "edge"}}
	b, _ := json.Marshal(model)
	return b
}

func TestSwitchBranches(t *testing.T) {
	r, err := Evaluate(context.Background(), switchGraph(), sample(t))
	if err != nil || r.Errors != 0 || r.Flagged != 1 {
		t.Fatal(r, err)
	}
}

func TestGraphRejections(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"duplicate node", func(m map[string]any) { m["nodes"] = append(m["nodes"].([]any), m["nodes"].([]any)[0]) }},
		{"missing node id", func(m map[string]any) { m["nodes"].([]any)[0].(map[string]any)["id"] = "" }},
		{"unsupported loader", func(m map[string]any) { m["nodes"].([]any)[1].(map[string]any)["type"] = "decisionNode" }},
		{"dangling edge", func(m map[string]any) { m["edges"].([]any)[0].(map[string]any)["targetId"] = "missing" }},
		{"duplicate edge", func(m map[string]any) { m["edges"] = append(m["edges"].([]any), m["edges"].([]any)[0]) }},
		{"cycle", func(m map[string]any) {
			m["edges"] = append(m["edges"].([]any), map[string]any{"id": "cycle", "sourceId": "branch0", "targetId": "rule"})
		}},
		{"output edge", func(m map[string]any) { m["edges"].([]any)[0].(map[string]any)["sourceId"] = "out" }},
		{"input edge", func(m map[string]any) { m["edges"].([]any)[0].(map[string]any)["targetId"] = "in" }},
		{"missing switch handle", func(m map[string]any) { m["edges"].([]any)[1].(map[string]any)["sourceHandle"] = "missing" }},
		{"disconnected branch", func(m map[string]any) { m["edges"] = append(m["edges"].([]any)[:2], m["edges"].([]any)[3:]...) }},
		{"wrong branch handle", func(m map[string]any) { m["edges"].([]any)[0].(map[string]any)["sourceHandle"] = "wrong" }},
		{"orphan", func(m map[string]any) {
			m["nodes"] = append(m["nodes"].([]any), map[string]any{"id": "orphan", "type": "outputNode"})
		}},
		{"second input", func(m map[string]any) {
			m["nodes"] = append(m["nodes"].([]any), map[string]any{"id": "input2", "type": "inputNode"})
		}},
		{"no output", func(m map[string]any) { m["nodes"].([]any)[2].(map[string]any)["type"] = "inputNode" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var m map[string]any
			_ = json.Unmarshal(switchGraph(), &m)
			tc.change(m)
			b, _ := json.Marshal(m)
			if err := ValidateModel(b); err == nil {
				t.Fatal("invalid graph accepted")
			}
		})
	}
	for _, model := range [][]byte{[]byte(`{}`), []byte("invalid"), []byte(strings.Repeat("x", 256*1024+1)), graph("expressionNode", map[string]any{"expressions": []any{}}), graph("functionNode", ""), graph("functionNode", 1), graph("switchNode", map[string]any{}), graph("decisionTableNode", map[string]any{})} {
		if err := ValidateModel(model); err == nil {
			t.Fatalf("invalid graph accepted %s", short(string(model)))
		}
	}
}

func TestDatasetBoundsAndTypes(t *testing.T) {
	for _, change := range []func(*Dataset){
		func(d *Dataset) { d.RowCount = 1 }, func(d *Dataset) { d.Columns[0].Type = "wrong" }, func(d *Dataset) { d.Columns[0].Name = "" }, func(d *Dataset) { delete(d.Rows[0], "ID") }, func(d *Dataset) { delete(d.Rows[0], "ID"); d.Rows[0]["extra"] = "x" }, func(d *Dataset) { d.Rows[0]["ID"] = []any{1} }, func(d *Dataset) { d.Rows[0]["ID"] = strings.Repeat("x", MaxBytes) },
	} {
		d := sample(t)
		change(&d)
		if ValidateDataset(d) == nil {
			t.Fatal("invalid dataset accepted")
		}
		if _, e := SelectColumns(d, []Column{{Name: "ID", Type: "string"}}); e == nil {
			t.Fatal("invalid source dataset selected")
		}
	}
	if _, e := ParseCSV(context.Background(), "x\n"+strings.Repeat("1\n", MaxRows+1)); e == nil {
		t.Fatal("row limit ignored")
	}
	if _, e := ParseCSV(context.Background(), strings.Repeat("x", MaxBytes+1)); e == nil {
		t.Fatal("byte limit ignored")
	}
	if _, e := ParseJSON(context.Background(), []byte(strings.Repeat("x", MaxBytes+1))); e == nil {
		t.Fatal("JSON byte limit ignored")
	}
	if _, e := FromRows(context.Background(), make([]string, 201), [][]string{{}}); e == nil {
		t.Fatal("column limit ignored")
	}
	d, e := ParseCSV(context.Background(), "empty,mixed,decimal,big,exponent\n,1,0.10,9007199254740993,1e999\n,no,1.12,1,2\n")
	if e != nil {
		t.Fatal(e)
	}
	if d.Columns[0].Type != "string" || d.Columns[1].Type != "string" || d.Columns[2].Type != "number" || d.Columns[3].Type != "string" || d.Columns[4].Type != "string" {
		t.Fatal(d.Columns)
	}
	d.Columns[0].Type = "number"
	typed, e := typedRow(d.Rows[0], d.Columns)
	if e != nil || typed["empty"] != nil {
		t.Fatal(typed, e)
	}
}
