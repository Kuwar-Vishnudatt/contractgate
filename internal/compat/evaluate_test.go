package compat

import (
	"testing"

	"github.com/Kuwar-Vishnudatt/contractgate/internal/contract"
)

func contractV(version string, mode contract.Mode, fields ...contract.Field) *contract.Contract {
	return &contract.Contract{Name: "t", Version: version, Compatibility: mode, Fields: fields}
}

func TestBreakingChangeWithoutMajorBumpIsBlocked(t *testing.T) {
	old := contractV("1.0.0", contract.ModeFull, f("id", contract.TypeString, true))
	new := contractV("1.1.0", contract.ModeFull, f("id", contract.TypeString, true), f("amount", contract.TypeNumber, true))
	d, err := Evaluate(old, new)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Blocked {
		t.Fatal("expected a new required field without a major bump to be blocked")
	}
}

func TestBreakingChangeWithMajorBumpIsAllowed(t *testing.T) {
	old := contractV("1.0.0", contract.ModeFull, f("id", contract.TypeString, true))
	new := contractV("2.0.0", contract.ModeFull, f("id", contract.TypeString, true), f("amount", contract.TypeNumber, true))
	d, err := Evaluate(old, new)
	if err != nil {
		t.Fatal(err)
	}
	if d.Blocked {
		t.Fatalf("expected a breaking change WITH a major bump to be allowed, reasons: %v", d.Reasons)
	}
}

func TestCompatibleChangeWithoutBumpIsAllowed(t *testing.T) {
	old := contractV("1.0.0", contract.ModeFull, f("id", contract.TypeString, true))
	new := contractV("1.0.0", contract.ModeFull, f("id", contract.TypeString, true), f("note", contract.TypeString, false))
	d, err := Evaluate(old, new)
	if err != nil {
		t.Fatal(err)
	}
	if d.Blocked {
		t.Fatalf("a non-breaking change must never be blocked regardless of version, reasons: %v", d.Reasons)
	}
}

func TestForwardOnlyModeIgnoresBackwardOnlyBreaks(t *testing.T) {
	// Adding a new required field breaks backward only. Under forward-only
	// enforcement, that specific break should NOT block.
	old := contractV("1.0.0", contract.ModeForward, f("id", contract.TypeString, true))
	new := contractV("1.1.0", contract.ModeForward, f("id", contract.TypeString, true), f("amount", contract.TypeNumber, true))
	d, err := Evaluate(old, new)
	if err != nil {
		t.Fatal(err)
	}
	if d.Blocked {
		t.Fatalf("forward-only mode must not block a backward-only-breaking change, reasons: %v", d.Reasons)
	}
}

func TestBackwardOnlyModeCatchesBackwardBreak(t *testing.T) {
	old := contractV("1.0.0", contract.ModeBackward, f("id", contract.TypeString, true))
	new := contractV("1.1.0", contract.ModeBackward, f("id", contract.TypeString, true), f("amount", contract.TypeNumber, true))
	d, err := Evaluate(old, new)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Blocked {
		t.Fatal("backward mode must catch a backward-breaking change")
	}
}

func TestBackwardOnlyModeIgnoresForwardOnlyBreaks(t *testing.T) {
	// Removing a required field breaks forward only.
	old := contractV("1.0.0", contract.ModeBackward, f("id", contract.TypeString, true), f("amount", contract.TypeNumber, true))
	new := contractV("1.1.0", contract.ModeBackward, f("id", contract.TypeString, true))
	d, err := Evaluate(old, new)
	if err != nil {
		t.Fatal(err)
	}
	if d.Blocked {
		t.Fatalf("backward-only mode must not block a forward-only-breaking change, reasons: %v", d.Reasons)
	}
}

func TestModeNoneAllowsAnything(t *testing.T) {
	old := contractV("1.0.0", contract.ModeNone, f("id", contract.TypeString, true))
	new := contractV("1.0.0", contract.ModeNone, f("amount", contract.TypeNumber, true)) // id removed entirely, amount added
	d, err := Evaluate(old, new)
	if err != nil {
		t.Fatal(err)
	}
	if d.Blocked {
		t.Fatalf("mode none must allow any change, reasons: %v", d.Reasons)
	}
}

func TestWeakeningModeWithoutMajorBumpIsBlocked(t *testing.T) {
	old := contractV("1.0.0", contract.ModeFull, f("id", contract.TypeString, true))
	new := contractV("1.1.0", contract.ModeNone, f("id", contract.TypeString, true)) // same fields, mode weakened
	d, err := Evaluate(old, new)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Blocked {
		t.Fatal("expected weakening full->none without a major bump to be blocked, even with no field changes")
	}
	if !d.ModeWeakenedWithoutMajorBump {
		t.Fatal("expected ModeWeakenedWithoutMajorBump to be set")
	}
}

func TestWeakeningModeWithMajorBumpIsAllowed(t *testing.T) {
	old := contractV("1.0.0", contract.ModeFull, f("id", contract.TypeString, true))
	new := contractV("2.0.0", contract.ModeNone, f("id", contract.TypeString, true))
	d, err := Evaluate(old, new)
	if err != nil {
		t.Fatal(err)
	}
	if d.Blocked {
		t.Fatal("expected weakening mode WITH a major bump to be allowed")
	}
}

func TestStrengtheningModeNeverBlocks(t *testing.T) {
	old := contractV("1.0.0", contract.ModeNone, f("id", contract.TypeString, true))
	new := contractV("1.0.0", contract.ModeFull, f("id", contract.TypeString, true))
	d, err := Evaluate(old, new)
	if err != nil {
		t.Fatal(err)
	}
	if d.Blocked {
		t.Fatal("strengthening compatibility mode must never itself be blocked")
	}
}

func TestInvalidVersionStringReturnsError(t *testing.T) {
	old := contractV("not-a-version", contract.ModeFull, f("id", contract.TypeString, true))
	new := contractV("1.0.0", contract.ModeFull, f("id", contract.TypeString, true))
	if _, err := Evaluate(old, new); err == nil {
		t.Fatal("expected an error for an invalid old version string")
	}
}
