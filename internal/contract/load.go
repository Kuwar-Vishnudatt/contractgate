package contract

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Load reads and parses a contract YAML file, validating its basic shape
// (every field has a name and a known type; compatibility mode defaults to
// "full" if unset, matching the safest default rather than silently
// allowing any change).
func Load(path string) (*Contract, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read contract %s: %w", path, err)
	}
	var c Contract
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse contract %s: %w", path, err)
	}
	if c.Compatibility == "" {
		c.Compatibility = ModeFull
	}
	if err := validateShape(&c); err != nil {
		return nil, fmt.Errorf("invalid contract %s: %w", path, err)
	}
	return &c, nil
}

func validateShape(c *Contract) error {
	if c.Name == "" {
		return fmt.Errorf("missing name")
	}
	if c.Version == "" {
		return fmt.Errorf("missing version")
	}
	switch c.Compatibility {
	case ModeBackward, ModeForward, ModeFull, ModeNone:
	default:
		return fmt.Errorf("unknown compatibility mode %q", c.Compatibility)
	}
	seen := map[string]bool{}
	for _, f := range c.Fields {
		if f.Name == "" {
			return fmt.Errorf("field with empty name")
		}
		if seen[f.Name] {
			return fmt.Errorf("duplicate field %q", f.Name)
		}
		seen[f.Name] = true
		switch f.Type {
		case TypeString, TypeInteger, TypeNumber, TypeBoolean:
		default:
			return fmt.Errorf("field %q: unknown type %q", f.Name, f.Type)
		}
		if len(f.Enum) > 0 && f.Type != TypeString {
			return fmt.Errorf("field %q: enum is only supported on string fields", f.Name)
		}
	}
	return nil
}
