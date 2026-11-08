// Package compat classifies a contract change into exactly which
// compatibility directions it breaks, mirroring the reasoning real schema
// registries (Confluent et al.) use:
//
//   - BACKWARD compatibility: can a consumer built against the NEW contract
//     still correctly read data produced under the OLD contract? This is
//     what matters when consumers upgrade before producers, or when
//     replaying historical data against new consumer code.
//   - FORWARD compatibility: can a consumer built against the OLD contract
//     still correctly read data produced under the NEW contract? This is
//     what matters when producers upgrade before all consumers do.
//
// The two are NOT mirror images of each other in the way it's easy to
// assume — "making a required field optional" breaks forward compatibility
// but not backward, and "making an optional field required" is exactly the
// reverse. Getting the direction right for each kind of change is the
// entire point of this package; see compat_test.go for each case nailed
// down independently in both directions.
package compat

import (
	"fmt"

	"github.com/Kuwar-Vishnudatt/contractgate/internal/contract"
)

// IssueKind categorizes one detected compatibility problem.
type IssueKind string

const (
	IssueNewRequiredField  IssueKind = "new_required_field"
	IssueRemovedField      IssueKind = "removed_field"
	IssueTypeChanged       IssueKind = "type_changed"
	IssueBecameRequired    IssueKind = "became_required"
	IssueBecameOptional    IssueKind = "became_optional"
	IssueEnumNarrowed      IssueKind = "enum_narrowed"
	IssueEnumRelaxedToOpen IssueKind = "enum_relaxed_to_unrestricted"
	IssueEnumRestricted    IssueKind = "enum_restricted_from_unrestricted"
)

// Issue is one detected problem in one direction.
type Issue struct {
	Field   string
	Kind    IssueKind
	Message string
}

// Report is the full bidirectional classification of a contract change.
type Report struct {
	BackwardIssues []Issue
	ForwardIssues  []Issue
}

func (r Report) BreaksBackward() bool { return len(r.BackwardIssues) > 0 }
func (r Report) BreaksForward() bool  { return len(r.ForwardIssues) > 0 }

// Classify compares old and new field-by-field and returns every
// compatibility issue found, in both directions. It does not consult the
// contract's declared compatibility Mode — that's applied by Evaluate,
// which decides what to DO with this Report.
func Classify(oldC, newC *contract.Contract) Report {
	var r Report

	oldFields := map[string]*contract.Field{}
	for i := range oldC.Fields {
		oldFields[oldC.Fields[i].Name] = &oldC.Fields[i]
	}
	newFields := map[string]*contract.Field{}
	for i := range newC.Fields {
		newFields[newC.Fields[i].Name] = &newC.Fields[i]
	}

	for name, nf := range newFields {
		of, existedBefore := oldFields[name]
		if !existedBefore {
			classifyAddedField(&r, nf)
			continue
		}
		classifyExistingField(&r, of, nf)
	}

	for name, of := range oldFields {
		if _, stillPresent := newFields[name]; !stillPresent {
			classifyRemovedField(&r, of)
		}
	}

	return r
}

func classifyAddedField(r *Report, nf *contract.Field) {
	if nf.Required {
		// Old data never had this field at all, and the new contract
		// demands it: a new-schema consumer reading old data would find it
		// missing. Forward is fine — an old-schema consumer reading new
		// data simply doesn't look for a field it's never heard of.
		r.BackwardIssues = append(r.BackwardIssues, Issue{
			Field: nf.Name, Kind: IssueNewRequiredField,
			Message: fmt.Sprintf("field %q is new and required, but old data never had it", nf.Name),
		})
	}
}

func classifyRemovedField(r *Report, of *contract.Field) {
	// Backward is fine regardless: a new-schema consumer simply doesn't
	// look for a field that no longer exists in the contract, whether or
	// not old data happened to carry it.
	if of.Required {
		// Forward breaks ONLY if the field used to be required: an
		// old-schema consumer still expects it on every record, but new
		// data no longer carries it at all. If it was merely optional,
		// an old-schema consumer already tolerated its absence.
		r.ForwardIssues = append(r.ForwardIssues, Issue{
			Field: of.Name, Kind: IssueRemovedField,
			Message: fmt.Sprintf("required field %q was removed; old consumers still expect it on every record", of.Name),
		})
	}
}

func classifyExistingField(r *Report, of, nf *contract.Field) {
	if of.Type != nf.Type {
		msg := fmt.Sprintf("field %q changed type from %s to %s", nf.Name, of.Type, nf.Type)
		r.BackwardIssues = append(r.BackwardIssues, Issue{Field: nf.Name, Kind: IssueTypeChanged, Message: msg})
		r.ForwardIssues = append(r.ForwardIssues, Issue{Field: nf.Name, Kind: IssueTypeChanged, Message: msg})
		return // a changed type makes required/enum comparisons meaningless
	}

	switch {
	case !of.Required && nf.Required:
		// Old data may have omitted this field (it was optional then); a
		// new-schema consumer now demands it on every record. Forward is
		// fine: an old-schema consumer only ever expected it "maybe," and
		// it being present on every new record still satisfies "maybe."
		r.BackwardIssues = append(r.BackwardIssues, Issue{
			Field: nf.Name, Kind: IssueBecameRequired,
			Message: fmt.Sprintf("field %q became required; old data may not have had it", nf.Name),
		})
	case of.Required && !nf.Required:
		// Backward is fine: old data always had it (it was required then),
		// which trivially still satisfies "optional" now. Forward breaks:
		// an old-schema consumer still expects it on every record, but new
		// data is now allowed to omit it.
		r.ForwardIssues = append(r.ForwardIssues, Issue{
			Field: nf.Name, Kind: IssueBecameOptional,
			Message: fmt.Sprintf("field %q became optional; old consumers still expect it on every record", nf.Name),
		})
	}

	if nf.Type == contract.TypeString {
		classifyEnumChange(r, of, nf)
	}
}

func classifyEnumChange(r *Report, of, nf *contract.Field) {
	oldOpen := len(of.Enum) == 0
	newOpen := len(nf.Enum) == 0

	switch {
	case oldOpen && newOpen:
		return
	case oldOpen && !newOpen:
		// Was unrestricted, now restricted to a specific set. Old data may
		// have used any string value, quite possibly one outside the new
		// restricted set: backward breaks. New data, constrained to the
		// new (narrower) set, is still just "some string" to an
		// unrestricted old-schema consumer: forward is fine.
		r.BackwardIssues = append(r.BackwardIssues, Issue{
			Field: nf.Name, Kind: IssueEnumRestricted,
			Message: fmt.Sprintf("field %q went from unrestricted to enum %v; old data may use values outside it", nf.Name, nf.Enum),
		})
	case !oldOpen && newOpen:
		// Was restricted, now unrestricted. Backward is fine: old data's
		// values were all within the old enum, which is certainly still
		// "some string" under the new unrestricted field. Forward breaks:
		// new data may now use any string, including values the old,
		// still-restricted-enum consumer would reject.
		r.ForwardIssues = append(r.ForwardIssues, Issue{
			Field: nf.Name, Kind: IssueEnumRelaxedToOpen,
			Message: fmt.Sprintf("field %q went from enum %v to unrestricted; new data may use values old consumers reject", nf.Name, of.Enum),
		})
	default:
		oldSet := toSet(of.Enum)
		newSet := toSet(nf.Enum)
		var removed, added []string
		for _, v := range of.Enum {
			if !newSet[v] {
				removed = append(removed, v)
			}
		}
		for _, v := range nf.Enum {
			if !oldSet[v] {
				added = append(added, v)
			}
		}
		if len(removed) > 0 {
			// A value old data might have used is no longer allowed: a
			// new-schema consumer could reject old data outright.
			r.BackwardIssues = append(r.BackwardIssues, Issue{
				Field: nf.Name, Kind: IssueEnumNarrowed,
				Message: fmt.Sprintf("field %q enum removed value(s) %v; old data may have used them", nf.Name, removed),
			})
		}
		if len(added) > 0 {
			// New data might use a value that didn't exist in the old
			// enum: an old-schema consumer could reject new data outright.
			r.ForwardIssues = append(r.ForwardIssues, Issue{
				Field: nf.Name, Kind: IssueEnumNarrowed,
				Message: fmt.Sprintf("field %q enum added value(s) %v; old consumers don't know them", nf.Name, added),
			})
		}
	}
}

func toSet(values []string) map[string]bool {
	m := make(map[string]bool, len(values))
	for _, v := range values {
		m[v] = true
	}
	return m
}
