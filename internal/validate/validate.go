// Package validate enforces a contract against actual event data at
// ingestion time — the runtime complement to package compat's CI-time
// schema-change checks. A contract with no CI-time violations is still
// useless if producers can send data that doesn't conform to it at all.
package validate

import (
	"fmt"

	"github.com/Kuwar-Vishnudatt/contractgate/internal/contract"
)

// Violation is one way a single event failed to conform to a contract.
type Violation struct {
	Field   string
	Message string
}

func (v Violation) String() string { return fmt.Sprintf("%s: %s", v.Field, v.Message) }

// Event validates one decoded JSON object (as produced by encoding/json
// unmarshaling into map[string]any) against a contract. Unknown fields not
// declared in the contract are allowed and ignored — this project's
// contracts describe a required MINIMUM shape, not a closed one; rejecting
// unknown fields would make additive evolution impossible to roll out
// gradually across producers.
func Event(c *contract.Contract, data map[string]any) []Violation {
	var violations []Violation

	for _, field := range c.Fields {
		raw, present := data[field.Name]
		if !present || raw == nil {
			if field.Required {
				violations = append(violations, Violation{field.Name, "required field is missing"})
			}
			continue
		}

		if v := checkType(field, raw); v != "" {
			violations = append(violations, Violation{field.Name, v})
			continue
		}

		if len(field.Enum) > 0 && field.Type == contract.TypeString {
			s := raw.(string)
			if !containsString(field.Enum, s) {
				violations = append(violations, Violation{
					field.Name, fmt.Sprintf("value %q is not one of the allowed enum values %v", s, field.Enum),
				})
			}
		}
	}

	return violations
}

func checkType(field contract.Field, raw any) string {
	switch field.Type {
	case contract.TypeString:
		if _, ok := raw.(string); !ok {
			return fmt.Sprintf("expected type string, got %T", raw)
		}
	case contract.TypeBoolean:
		if _, ok := raw.(bool); !ok {
			return fmt.Sprintf("expected type boolean, got %T", raw)
		}
	case contract.TypeNumber:
		if _, ok := raw.(float64); !ok {
			return fmt.Sprintf("expected type number, got %T", raw)
		}
	case contract.TypeInteger:
		n, ok := raw.(float64)
		if !ok {
			return fmt.Sprintf("expected type integer, got %T", raw)
		}
		if n != float64(int64(n)) {
			return fmt.Sprintf("expected an integer value, got %v (has a fractional part)", n)
		}
	}
	return ""
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
