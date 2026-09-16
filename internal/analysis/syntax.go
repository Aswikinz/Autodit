package analysis

import (
	"encoding/json"
	"fmt"

	zen "github.com/gorules/zen-go/v2"
)

// Native decision tables skip invalid cells. Preflight makes malformed rules
// visible instead of returning a misleading successful result with no flags.
// This function is invoked only in the resource-limited evaluation process.
func validateSyntax(model []byte) error {
	var graph struct {
		Nodes []graphNode `json:"nodes"`
	}
	if err := json.Unmarshal(model, &graph); err != nil {
		return err
	}
	seen := map[string]bool{}
	check := func(expression string, unary bool, location string) error {
		if expression == "" {
			return nil
		}
		key := fmt.Sprint(unary) + "\x00" + expression
		if seen[key] {
			return nil
		}
		seen[key] = true
		context := map[string]any{"data": map[string]any{}, "$": nil}
		var err error
		if unary {
			_, err = zen.EvaluateUnaryExpression(expression, context)
		} else {
			_, err = zen.EvaluateExpression[any](expression, context)
		}
		if err == nil {
			return nil
		}
		var detail struct {
			Type   string `json:"type"`
			Source string `json:"source"`
		}
		if json.Unmarshal([]byte(err.Error()), &detail) != nil {
			return fmt.Errorf("%s: expression could not be checked", location)
		}
		switch detail.Type {
		case "lexerError", "parserError", "compilerError":
			return fmt.Errorf("%s: invalid expression (%s)", location, short(detail.Source))
		default:
			return nil // Runtime types depend on each row and earlier graph nodes.
		}
	}
	for _, node := range graph.Nodes {
		location := fmt.Sprintf("node %q", label(node.Name, node.ID))
		switch node.Type {
		case "decisionTableNode":
			var c struct {
				InputField string `json:"inputField"`
				Inputs     []struct {
					ID           string  `json:"id"`
					Name         string  `json:"name"`
					Field        *string `json:"field"`
					DefaultValue string  `json:"defaultValue"`
				} `json:"inputs"`
				Outputs []struct {
					ID           string `json:"id"`
					Name         string `json:"name"`
					DefaultValue string `json:"defaultValue"`
				} `json:"outputs"`
				Rules []map[string]string `json:"rules"`
			}
			if err := json.Unmarshal(node.Content, &c); err != nil {
				return fmt.Errorf("%s: invalid decision table", location)
			}
			if err := check(c.InputField, false, location+" input"); err != nil {
				return err
			}
			for _, column := range c.Inputs {
				columnLocation := location + " column " + label(column.Name, column.ID)
				if column.Field != nil {
					if *column.Field == "" {
						return fmt.Errorf("%s: set a field or use a null field for a full expression", columnLocation)
					}
					if err := check(*column.Field, false, columnLocation+" field"); err != nil {
						return err
					}
				}
				if err := check(column.DefaultValue, false, columnLocation+" default"); err != nil {
					return err
				}
				for i, row := range c.Rules {
					if err := check(row[column.ID], column.Field != nil, fmt.Sprintf("%s rule %d", columnLocation, i+1)); err != nil {
						return err
					}
				}
			}
			for _, column := range c.Outputs {
				columnLocation := location + " output " + label(column.Name, column.ID)
				if err := check(column.DefaultValue, false, columnLocation+" default"); err != nil {
					return err
				}
				for i, row := range c.Rules {
					if err := check(row[column.ID], false, fmt.Sprintf("%s rule %d", columnLocation, i+1)); err != nil {
						return err
					}
				}
			}
		case "expressionNode":
			var c struct {
				InputField  string                       `json:"inputField"`
				Expressions []struct{ ID, Value string } `json:"expressions"`
			}
			if err := json.Unmarshal(node.Content, &c); err != nil {
				return fmt.Errorf("%s: invalid expressions", location)
			}
			if err := check(c.InputField, false, location+" input"); err != nil {
				return err
			}
			for _, expression := range c.Expressions {
				if err := check(expression.Value, false, location+" expression "+expression.ID); err != nil {
					return err
				}
			}
		case "switchNode":
			var c struct {
				Statements []struct {
					ID, Condition string
					Default       bool `json:"isDefault"`
				} `json:"statements"`
			}
			if err := json.Unmarshal(node.Content, &c); err != nil {
				return fmt.Errorf("%s: invalid switch", location)
			}
			for _, statement := range c.Statements {
				if !statement.Default {
					if err := check(statement.Condition, false, location+" branch "+statement.ID); err != nil {
						return err
					}
				}
			}
		}
	}
	engine := zen.NewEngine(zen.EngineConfig{})
	defer engine.Dispose()
	decision, err := engine.CreateDecision(model)
	if err != nil {
		return fmt.Errorf("decision could not be compiled: %s", short(err.Error()))
	}
	decision.Dispose()
	return nil
}

func label(name, id string) string {
	if name != "" {
		return short(name)
	}
	return short(id)
}
