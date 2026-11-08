package contract

import (
	"os"
	"path/filepath"
	"testing"
)

func writeContract(t *testing.T, yaml string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "contract.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadValidContract(t *testing.T) {
	path := writeContract(t, `
name: orders
version: 1.0.0
compatibility: full
fields:
  - name: order_id
    type: string
    required: true
  - name: status
    type: string
    required: true
    enum: [placed, shipped]
`)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "orders" || c.Version != "1.0.0" || c.Compatibility != ModeFull {
		t.Fatalf("got %+v", c)
	}
	if len(c.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(c.Fields))
	}
}

func TestLoadDefaultsToFullCompatibilityWhenUnset(t *testing.T) {
	path := writeContract(t, "name: orders\nversion: 1.0.0\nfields: []\n")
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Compatibility != ModeFull {
		t.Fatalf("expected default compatibility mode 'full', got %q", c.Compatibility)
	}
}

func TestLoadRejectsMissingName(t *testing.T) {
	path := writeContract(t, "version: 1.0.0\nfields: []\n")
	if _, err := Load(path); err == nil {
		t.Fatal("expected an error for a contract missing a name")
	}
}

func TestLoadRejectsDuplicateFieldNames(t *testing.T) {
	path := writeContract(t, `
name: orders
version: 1.0.0
fields:
  - name: id
    type: string
    required: true
  - name: id
    type: string
    required: false
`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected an error for duplicate field names")
	}
}

func TestLoadRejectsUnknownType(t *testing.T) {
	path := writeContract(t, "name: orders\nversion: 1.0.0\nfields:\n  - name: id\n    type: bignum\n    required: true\n")
	if _, err := Load(path); err == nil {
		t.Fatal("expected an error for an unknown field type")
	}
}

func TestLoadRejectsEnumOnNonStringField(t *testing.T) {
	path := writeContract(t, "name: orders\nversion: 1.0.0\nfields:\n  - name: amount\n    type: number\n    required: true\n    enum: [a, b]\n")
	if _, err := Load(path); err == nil {
		t.Fatal("expected an error for an enum on a non-string field")
	}
}

func TestLoadRejectsUnknownCompatibilityMode(t *testing.T) {
	path := writeContract(t, "name: orders\nversion: 1.0.0\ncompatibility: sideways\nfields: []\n")
	if _, err := Load(path); err == nil {
		t.Fatal("expected an error for an unknown compatibility mode")
	}
}

func TestFieldByName(t *testing.T) {
	c := &Contract{Fields: []Field{{Name: "a"}, {Name: "b"}}}
	if c.FieldByName("b") == nil {
		t.Fatal("expected to find field b")
	}
	if c.FieldByName("missing") != nil {
		t.Fatal("expected nil for a field that doesn't exist")
	}
}
