package analysis

import (
	"encoding/json"
	"errors"
	"fmt"
)

type graphNode struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Content json.RawMessage `json:"content"`
}
type graphEdge struct {
	ID           string `json:"id"`
	SourceID     string `json:"sourceId"`
	TargetID     string `json:"targetId"`
	SourceHandle string `json:"sourceHandle"`
}

// ValidateModel checks complete executable graphs independently of the canvas.
// Referenced decisions and custom nodes require loaders not installed in this workspace.
func ValidateModel(data []byte) error {
	if len(data) > 256*1024 {
		return errors.New("decision graph exceeds 256 KiB")
	}
	var g struct {
		Nodes []graphNode `json:"nodes"`
		Edges []graphEdge `json:"edges"`
	}
	if json.Unmarshal(data, &g) != nil || len(g.Nodes) < 2 || len(g.Nodes) > 32 || len(g.Edges) == 0 || len(g.Edges) > 64 {
		return errors.New("connect a Request and Response using at most 32 nodes and 64 connections")
	}
	nodes := map[string]graphNode{}
	branches := map[string]map[string]bool{}
	inputs := []string{}
	outputs := map[string]bool{}
	for _, n := range g.Nodes {
		if n.ID == "" || len(n.ID) > 256 {
			return errors.New("every node needs an identifier")
		}
		if _, exists := nodes[n.ID]; exists {
			return errors.New("node identifiers must be unique")
		}
		nodes[n.ID] = n
		switch n.Type {
		case "inputNode":
			inputs = append(inputs, n.ID)
		case "outputNode":
			outputs[n.ID] = true
		case "decisionTableNode":
			if err := validateTable(n.Content); err != nil {
				return err
			}
		case "expressionNode":
			var c struct {
				Expressions []struct{ ID, Key, Value string } `json:"expressions"`
			}
			if json.Unmarshal(n.Content, &c) != nil || len(c.Expressions) == 0 || len(c.Expressions) > 200 {
				return errors.New("expression nodes need between 1 and 200 expressions")
			}
			seen := map[string]bool{}
			for _, x := range c.Expressions {
				if x.ID == "" || seen[x.ID] || x.Key == "" || len(x.Key) > 256 || x.Value == "" || len(x.Value) > 4096 {
					return errors.New("expressions need unique identifiers, output names and values up to 4096 characters")
				}
				seen[x.ID] = true
			}
		case "functionNode":
			var source string
			if json.Unmarshal(n.Content, &source) != nil {
				var c struct {
					Source string `json:"source"`
				}
				if json.Unmarshal(n.Content, &c) != nil {
					return errors.New("function source is invalid")
				}
				source = c.Source
			}
			if len(source) == 0 || len(source) > 65536 {
				return errors.New("function source must contain between 1 and 65,536 characters")
			}
		case "switchNode":
			var c struct {
				HitPolicy  string `json:"hitPolicy"`
				Statements []struct {
					ID        string `json:"id"`
					Condition string `json:"condition"`
					Default   bool   `json:"isDefault"`
				} `json:"statements"`
			}
			if json.Unmarshal(n.Content, &c) != nil || (c.HitPolicy != "first" && c.HitPolicy != "collect") || len(c.Statements) == 0 || len(c.Statements) > 32 {
				return errors.New("switch needs a first or collect policy and between 1 and 32 branches")
			}
			branches[n.ID] = map[string]bool{}
			defaultSeen := false
			for _, s := range c.Statements {
				if s.ID == "" || branches[n.ID][s.ID] || len(s.Condition) > 4096 || (!s.Default && s.Condition == "") || (s.Default && defaultSeen) {
					return errors.New("switch branches need unique identifiers, conditions and at most one default")
				}
				branches[n.ID][s.ID] = true
				defaultSeen = defaultSeen || s.Default
			}
		default:
			return fmt.Errorf("node type %q is unavailable; use Request, Response, Decision table, Expression, Function or Switch", n.Type)
		}
	}
	if len(inputs) != 1 || len(outputs) == 0 {
		return errors.New("graph needs exactly one Request and at least one Response")
	}
	forward := map[string][]string{}
	backward := map[string][]string{}
	ids := map[string]bool{}
	links := map[string]bool{}
	usedBranches := map[string]bool{}
	for _, e := range g.Edges {
		source, sourceOK := nodes[e.SourceID]
		target, targetOK := nodes[e.TargetID]
		if e.ID == "" || ids[e.ID] || !sourceOK || !targetOK {
			return errors.New("connections need unique identifiers and existing nodes")
		}
		ids[e.ID] = true
		if source.Type == "outputNode" || target.Type == "inputNode" {
			return errors.New("Request must start a path and Response must end it")
		}
		link := e.SourceID + "\x00" + e.SourceHandle + "\x00" + e.TargetID
		if links[link] {
			return errors.New("duplicate connection")
		}
		links[link] = true
		if source.Type == "switchNode" {
			if !branches[e.SourceID][e.SourceHandle] {
				return errors.New("switch connection refers to a missing branch")
			}
			usedBranches[e.SourceID+"\x00"+e.SourceHandle] = true
		} else if e.SourceHandle != "" {
			return errors.New("only switch connections can select a branch")
		}
		forward[e.SourceID] = append(forward[e.SourceID], e.TargetID)
		backward[e.TargetID] = append(backward[e.TargetID], e.SourceID)
	}
	for node, handles := range branches {
		for handle := range handles {
			if !usedBranches[node+"\x00"+handle] {
				return errors.New("connect every switch branch to a Response path")
			}
		}
	}
	colors := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		if colors[id] == 1 {
			return errors.New("decision graph contains a cycle")
		}
		if colors[id] == 2 {
			return nil
		}
		colors[id] = 1
		for _, next := range forward[id] {
			if err := visit(next); err != nil {
				return err
			}
		}
		colors[id] = 2
		return nil
	}
	if err := visit(inputs[0]); err != nil {
		return err
	}
	reachesOutput := map[string]bool{}
	var mark func(string)
	mark = func(id string) {
		if reachesOutput[id] {
			return
		}
		reachesOutput[id] = true
		for _, prev := range backward[id] {
			mark(prev)
		}
	}
	for out := range outputs {
		mark(out)
	}
	for id := range nodes {
		if colors[id] != 2 || !reachesOutput[id] {
			return errors.New("every node must connect from Request to a Response")
		}
	}
	return nil
}

func validateTable(data []byte) error {
	var c struct {
		HitPolicy string                       `json:"hitPolicy"`
		Inputs    []struct{ ID, Field string } `json:"inputs"`
		Outputs   []struct{ ID, Field string } `json:"outputs"`
		Rules     []map[string]string          `json:"rules"`
	}
	if json.Unmarshal(data, &c) != nil || (c.HitPolicy != "first" && c.HitPolicy != "collect") || len(c.Inputs) > 200 || len(c.Outputs) == 0 || len(c.Outputs) > 200 || len(c.Rules) == 0 || len(c.Rules) > 500 {
		return errors.New("decision table needs a first or collect policy, 1 to 500 rules and 1 to 200 outputs")
	}
	ids := map[string]bool{}
	for _, col := range append(c.Inputs, c.Outputs...) {
		if col.ID == "" || col.ID == "_id" || ids[col.ID] || len(col.Field) > 4096 {
			return errors.New("decision table columns need unique identifiers and valid field expressions")
		}
		ids[col.ID] = true
	}
	for _, out := range c.Outputs {
		if out.Field == "" {
			return errors.New("decision table outputs need field names")
		}
	}
	for _, row := range c.Rules {
		for key, value := range row {
			if len(value) > 4096 || (!ids[key] && key != "_id" && key != "_description") {
				return errors.New("decision table rule refers to an unknown column or is too long")
			}
		}
	}
	return nil
}
