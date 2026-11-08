package compat

import (
	"fmt"

	"github.com/Kuwar-Vishnudatt/contractgate/internal/contract"
	"github.com/Kuwar-Vishnudatt/contractgate/internal/semver"
)

// modeStrictness ranks compatibility modes from least to most protective,
// used to detect a producer quietly weakening their own declared mode
// (e.g. full -> none) as a way to dodge the gate rather than genuinely
// fixing a breaking change.
var modeStrictness = map[contract.Mode]int{
	contract.ModeNone:     0,
	contract.ModeForward:  1,
	contract.ModeBackward: 1,
	contract.ModeFull:     2,
}

// Decision is the final CI-gate verdict for one contract change.
type Decision struct {
	Report Report

	// EffectiveMode is the mode enforced for this check — the OLD
	// contract's declared mode, since that's what existing consumers were
	// already relying on.
	EffectiveMode contract.Mode

	RelevantIssues []Issue // the subset of Report's issues that matter under EffectiveMode

	ModeWeakenedWithoutMajorBump bool
	MajorBump                    bool

	Blocked bool
	Reasons []string
}

// Evaluate runs Classify and applies the old contract's declared
// compatibility mode plus the semver major-bump rule to produce a final
// pass/fail decision.
func Evaluate(oldC, newC *contract.Contract) (Decision, error) {
	oldV, err := semver.Parse(oldC.Version)
	if err != nil {
		return Decision{}, fmt.Errorf("old contract: %w", err)
	}
	newV, err := semver.Parse(newC.Version)
	if err != nil {
		return Decision{}, fmt.Errorf("new contract: %w", err)
	}

	report := Classify(oldC, newC)
	mode := oldC.Compatibility
	d := Decision{
		Report:        report,
		EffectiveMode: mode,
		MajorBump:     semver.IsMajorBump(oldV, newV),
	}

	switch mode {
	case contract.ModeBackward:
		d.RelevantIssues = report.BackwardIssues
	case contract.ModeForward:
		d.RelevantIssues = report.ForwardIssues
	case contract.ModeFull:
		d.RelevantIssues = append(append([]Issue{}, report.BackwardIssues...), report.ForwardIssues...)
	case contract.ModeNone:
		d.RelevantIssues = nil
	}

	if modeStrictness[newC.Compatibility] < modeStrictness[mode] && !d.MajorBump {
		d.ModeWeakenedWithoutMajorBump = true
		d.Blocked = true
		d.Reasons = append(d.Reasons, fmt.Sprintf(
			"compatibility mode weakened from %q to %q without a major version bump (%s -> %s)",
			mode, newC.Compatibility, oldC.Version, newC.Version))
	}

	if len(d.RelevantIssues) > 0 && !d.MajorBump {
		d.Blocked = true
		for _, issue := range d.RelevantIssues {
			d.Reasons = append(d.Reasons, fmt.Sprintf("[%s] %s", issue.Kind, issue.Message))
		}
	}

	return d, nil
}
