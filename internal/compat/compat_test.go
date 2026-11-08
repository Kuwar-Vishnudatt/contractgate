package compat

import (
	"testing"

	"github.com/Kuwar-Vishnudatt/contractgate/internal/contract"
)

func c(fields ...contract.Field) *contract.Contract {
	return &contract.Contract{Name: "t", Version: "1.0.0", Compatibility: contract.ModeFull, Fields: fields}
}

func f(name string, typ contract.FieldType, required bool, enum ...string) contract.Field {
	return contract.Field{Name: name, Type: typ, Required: required, Enum: enum}
}

// --- adding a field ---

func TestAddingOptionalFieldIsFullyCompatible(t *testing.T) {
	old := c(f("id", contract.TypeString, true))
	new := c(f("id", contract.TypeString, true), f("note", contract.TypeString, false))
	r := Classify(old, new)
	if r.BreaksBackward() || r.BreaksForward() {
		t.Fatalf("adding an optional field must break neither direction, got %+v", r)
	}
}

func TestAddingRequiredFieldBreaksBackwardOnly(t *testing.T) {
	old := c(f("id", contract.TypeString, true))
	new := c(f("id", contract.TypeString, true), f("amount", contract.TypeNumber, true))
	r := Classify(old, new)
	if !r.BreaksBackward() {
		t.Fatal("adding a new required field must break backward compat (old data lacks it)")
	}
	if r.BreaksForward() {
		t.Fatal("adding a new required field must NOT break forward compat (old schema ignores unknown fields)")
	}
}

// --- removing a field ---

func TestRemovingOptionalFieldIsFullyCompatible(t *testing.T) {
	old := c(f("id", contract.TypeString, true), f("note", contract.TypeString, false))
	new := c(f("id", contract.TypeString, true))
	r := Classify(old, new)
	if r.BreaksBackward() || r.BreaksForward() {
		t.Fatalf("removing an optional field must break neither direction, got %+v", r)
	}
}

func TestRemovingRequiredFieldBreaksForwardOnly(t *testing.T) {
	old := c(f("id", contract.TypeString, true), f("amount", contract.TypeNumber, true))
	new := c(f("id", contract.TypeString, true))
	r := Classify(old, new)
	if r.BreaksBackward() {
		t.Fatal("removing a required field must NOT break backward compat (new schema just doesn't look for it)")
	}
	if !r.BreaksForward() {
		t.Fatal("removing a required field must break forward compat (old consumers still expect it on every record)")
	}
}

// --- required <-> optional on an existing field ---

func TestOptionalBecomingRequiredBreaksBackwardOnly(t *testing.T) {
	old := c(f("amount", contract.TypeNumber, false))
	new := c(f("amount", contract.TypeNumber, true))
	r := Classify(old, new)
	if !r.BreaksBackward() {
		t.Fatal("optional->required must break backward (old data may have omitted it)")
	}
	if r.BreaksForward() {
		t.Fatal("optional->required must NOT break forward (old consumers only ever expected it maybe)")
	}
}

func TestRequiredBecomingOptionalBreaksForwardOnly(t *testing.T) {
	old := c(f("amount", contract.TypeNumber, true))
	new := c(f("amount", contract.TypeNumber, false))
	r := Classify(old, new)
	if r.BreaksBackward() {
		t.Fatal("required->optional must NOT break backward (old data always had it anyway)")
	}
	if !r.BreaksForward() {
		t.Fatal("required->optional must break forward (old consumers still expect it on every record)")
	}
}

// --- type changes ---

func TestTypeChangeBreaksBothDirections(t *testing.T) {
	old := c(f("amount", contract.TypeInteger, true))
	new := c(f("amount", contract.TypeString, true))
	r := Classify(old, new)
	if !r.BreaksBackward() || !r.BreaksForward() {
		t.Fatalf("a type change must break both directions, got %+v", r)
	}
}

func TestIdenticalContractsHaveNoIssues(t *testing.T) {
	old := c(f("id", contract.TypeString, true), f("amount", contract.TypeNumber, false))
	new := c(f("id", contract.TypeString, true), f("amount", contract.TypeNumber, false))
	r := Classify(old, new)
	if r.BreaksBackward() || r.BreaksForward() {
		t.Fatalf("identical contracts must have zero issues, got %+v", r)
	}
}

// --- enum changes: the trickiest, most mirror-prone logic in the package ---

func TestEnumNarrowingBreaksBackwardOnly(t *testing.T) {
	old := c(f("status", contract.TypeString, true, "placed", "shipped", "delivered"))
	new := c(f("status", contract.TypeString, true, "placed", "shipped"))
	r := Classify(old, new)
	if !r.BreaksBackward() {
		t.Fatal("removing an enum value must break backward (old data may have used 'delivered')")
	}
	if r.BreaksForward() {
		t.Fatal("removing an enum value must NOT break forward (new data's subset is still valid old values)")
	}
}

func TestEnumExpansionBreaksForwardOnly(t *testing.T) {
	old := c(f("status", contract.TypeString, true, "placed", "shipped"))
	new := c(f("status", contract.TypeString, true, "placed", "shipped", "delivered"))
	r := Classify(old, new)
	if r.BreaksBackward() {
		t.Fatal("adding an enum value must NOT break backward (old data only ever used the old subset)")
	}
	if !r.BreaksForward() {
		t.Fatal("adding an enum value must break forward (new data may use 'delivered', which old consumers don't know)")
	}
}

func TestRestrictingPreviouslyUnrestrictedFieldBreaksBackwardOnly(t *testing.T) {
	old := c(f("status", contract.TypeString, true)) // no enum: unrestricted
	new := c(f("status", contract.TypeString, true, "placed", "shipped"))
	r := Classify(old, new)
	if !r.BreaksBackward() {
		t.Fatal("restricting an unrestricted field must break backward (old data could be any string)")
	}
	if r.BreaksForward() {
		t.Fatal("restricting an unrestricted field must NOT break forward (new data's subset is still 'some string' to old)")
	}
}

func TestRelaxingRestrictedFieldToUnrestrictedBreaksForwardOnly(t *testing.T) {
	old := c(f("status", contract.TypeString, true, "placed", "shipped"))
	new := c(f("status", contract.TypeString, true)) // enum removed entirely: unrestricted
	r := Classify(old, new)
	if r.BreaksBackward() {
		t.Fatal("relaxing to unrestricted must NOT break backward (old data's values are still valid)")
	}
	if !r.BreaksForward() {
		t.Fatal("relaxing to unrestricted must break forward (new data could use any value, old consumers may reject it)")
	}
}

func TestEnumUnchangedIsCompatible(t *testing.T) {
	old := c(f("status", contract.TypeString, true, "placed", "shipped"))
	new := c(f("status", contract.TypeString, true, "shipped", "placed")) // same set, different order
	r := Classify(old, new)
	if r.BreaksBackward() || r.BreaksForward() {
		t.Fatalf("reordering an identical enum set must not be flagged as a change, got %+v", r)
	}
}

func TestSimultaneousEnumAdditionAndRemovalFlagsBothDirections(t *testing.T) {
	old := c(f("status", contract.TypeString, true, "placed", "shipped"))
	new := c(f("status", contract.TypeString, true, "placed", "delivered")) // dropped "shipped", added "delivered"
	r := Classify(old, new)
	if !r.BreaksBackward() {
		t.Fatal("expected backward break from the removed 'shipped' value")
	}
	if !r.BreaksForward() {
		t.Fatal("expected forward break from the added 'delivered' value")
	}
}
