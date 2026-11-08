package validate

import (
	"testing"

	"github.com/Kuwar-Vishnudatt/contractgate/internal/contract"
)

func testContract() *contract.Contract {
	return &contract.Contract{
		Name:    "orders",
		Version: "1.0.0",
		Fields: []contract.Field{
			{Name: "order_id", Type: contract.TypeString, Required: true},
			{Name: "amount", Type: contract.TypeNumber, Required: true},
			{Name: "quantity", Type: contract.TypeInteger, Required: true},
			{Name: "status", Type: contract.TypeString, Required: true, Enum: []string{"placed", "shipped"}},
			{Name: "note", Type: contract.TypeString, Required: false},
		},
	}
}

func TestValidEventHasNoViolations(t *testing.T) {
	c := testContract()
	data := map[string]any{
		"order_id": "o1", "amount": 9.99, "quantity": float64(3), "status": "placed",
	}
	if v := Event(c, data); len(v) != 0 {
		t.Fatalf("expected no violations, got %v", v)
	}
}

func TestMissingRequiredFieldIsAViolation(t *testing.T) {
	c := testContract()
	data := map[string]any{"amount": 9.99, "quantity": float64(3), "status": "placed"}
	v := Event(c, data)
	if len(v) != 1 || v[0].Field != "order_id" {
		t.Fatalf("expected one violation for missing order_id, got %v", v)
	}
}

func TestMissingOptionalFieldIsNotAViolation(t *testing.T) {
	c := testContract()
	data := map[string]any{"order_id": "o1", "amount": 9.99, "quantity": float64(3), "status": "placed"}
	if v := Event(c, data); len(v) != 0 {
		t.Fatalf("expected no violations when only the optional field is absent, got %v", v)
	}
}

func TestWrongTypeIsAViolation(t *testing.T) {
	c := testContract()
	data := map[string]any{"order_id": "o1", "amount": "not a number", "quantity": float64(3), "status": "placed"}
	v := Event(c, data)
	if len(v) != 1 || v[0].Field != "amount" {
		t.Fatalf("expected one type violation for amount, got %v", v)
	}
}

func TestFractionalValueForIntegerFieldIsAViolation(t *testing.T) {
	c := testContract()
	data := map[string]any{"order_id": "o1", "amount": 9.99, "quantity": 3.5, "status": "placed"}
	v := Event(c, data)
	if len(v) != 1 || v[0].Field != "quantity" {
		t.Fatalf("expected one violation for fractional quantity, got %v", v)
	}
}

func TestWholeNumberFloatIsValidForIntegerField(t *testing.T) {
	// JSON numbers decode to float64 regardless of whether they had a
	// decimal point; 3.0 must be accepted for an integer field.
	c := testContract()
	data := map[string]any{"order_id": "o1", "amount": 9.99, "quantity": float64(3), "status": "placed"}
	if v := Event(c, data); len(v) != 0 {
		t.Fatalf("expected a whole-number float to satisfy an integer field, got %v", v)
	}
}

func TestEnumViolation(t *testing.T) {
	c := testContract()
	data := map[string]any{"order_id": "o1", "amount": 9.99, "quantity": float64(3), "status": "cancelled"}
	v := Event(c, data)
	if len(v) != 1 || v[0].Field != "status" {
		t.Fatalf("expected one enum violation for status, got %v", v)
	}
}

func TestUnknownFieldsAreIgnored(t *testing.T) {
	c := testContract()
	data := map[string]any{
		"order_id": "o1", "amount": 9.99, "quantity": float64(3), "status": "placed",
		"some_future_field": "whatever",
	}
	if v := Event(c, data); len(v) != 0 {
		t.Fatalf("expected unknown fields to be silently allowed, got %v", v)
	}
}

func TestExplicitNullForRequiredFieldIsAViolation(t *testing.T) {
	c := testContract()
	data := map[string]any{"order_id": "o1", "amount": nil, "quantity": float64(3), "status": "placed"}
	v := Event(c, data)
	if len(v) != 1 || v[0].Field != "amount" {
		t.Fatalf("expected an explicit null for a required field to violate, got %v", v)
	}
}

func TestMultipleViolationsAreAllReported(t *testing.T) {
	c := testContract()
	// missing order_id, missing amount, missing quantity, and an invalid
	// enum value for status: 4 distinct violations expected.
	data := map[string]any{"status": "cancelled"}
	v := Event(c, data)
	if len(v) != 4 {
		t.Fatalf("expected 4 violations (3 missing + 1 enum), got %d: %v", len(v), v)
	}
}
