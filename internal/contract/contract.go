// Package contract defines the data-contract format this project enforces:
// a flat (non-nested) field list with a required semver version and an
// explicit compatibility mode. It's deliberately simpler than full JSON
// Schema or Avro — the engineering effort here is in correctly reasoning
// about compatibility, not in parsing an existing schema format.
package contract

// FieldType is the set of primitive types a contract field can declare.
// "integer" is distinct from "number" deliberately: a producer sending
// 3.5 for a field declared "integer" is a real contract violation that a
// JSON-number-only type system would silently miss.
type FieldType string

const (
	TypeString  FieldType = "string"
	TypeInteger FieldType = "integer"
	TypeNumber  FieldType = "number"
	TypeBoolean FieldType = "boolean"
)

// Field is one field in a contract.
type Field struct {
	Name     string    `yaml:"name"`
	Type     FieldType `yaml:"type"`
	Required bool      `yaml:"required"`
	// Enum, if non-empty, restricts a string field to these values. Only
	// meaningful for Type == TypeString.
	Enum []string `yaml:"enum,omitempty"`
}

// Mode is which direction(s) of compatibility a contract's evolution must
// preserve, mirroring Confluent Schema Registry's compatibility modes.
type Mode string

const (
	// ModeBackward: a consumer built against the NEW contract must still be
	// able to correctly read data produced under the OLD contract. This is
	// the mode that matters when consumers upgrade before producers (or
	// when replaying old data against new consumer code).
	ModeBackward Mode = "backward"
	// ModeForward: a consumer built against the OLD contract must still be
	// able to correctly read data produced under the NEW contract. This is
	// the mode that matters when producers upgrade before all consumers do.
	ModeForward Mode = "forward"
	// ModeFull requires both — the default, and the safest choice when you
	// don't control the rollout order of producers vs. consumers.
	ModeFull Mode = "full"
	// ModeNone performs no compatibility enforcement at all (any change is
	// allowed). Exists for contracts that genuinely have no external
	// consumers yet; using it to bypass a real violation is a misuse of the
	// escape hatch, not a bug in the gate.
	ModeNone Mode = "none"
)

// Contract is one versioned data contract.
type Contract struct {
	Name          string  `yaml:"name"`
	Version       string  `yaml:"version"` // semver, e.g. "1.2.0"
	Compatibility Mode    `yaml:"compatibility"`
	Fields        []Field `yaml:"fields"`
}

// FieldByName returns the field with the given name, or nil.
func (c *Contract) FieldByName(name string) *Field {
	for i := range c.Fields {
		if c.Fields[i].Name == name {
			return &c.Fields[i]
		}
	}
	return nil
}
