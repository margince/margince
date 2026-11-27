// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// callOutcomeOf is the stored outcome of a call as the store's plain string,
// nil when it has none.
func callOutcomeOf(a crmcontracts.Activity) *string {
	if a.CallOutcome == nil {
		return nil
	}
	s := string(*a.CallOutcome)
	return &s
}

// patchedCallOutcome is the outcome the row holds after the patch. A clear
// leaves none, a named outcome replaces it, and otherwise the stored one stays.
func (in UpdateActivityInput) patchedCallOutcome(current crmcontracts.Activity) *string {
	if in.clears(fieldCallOutcome) {
		return nil
	}
	return firstSet(in.CallOutcome, callOutcomeOf(current))
}

// clearableActivityColumns is what a restore (Undo) may set back to nothing on
// an activity, and what the row holds there now. Only the call outcome: undoing
// a first pick puts the call back to "no outcome". compose's clearableFields
// names the same list, held equal by gates/clearablefields_test.go.
func clearableActivityColumns(current crmcontracts.Activity) map[string]storekit.Clearable {
	return map[string]storekit.Clearable{
		"call_outcome": {Column: "call_outcome", Current: callOutcomeOf(current)},
	}
}

// clearsDeclared is a clear of a field the store declares clearable.
func (in UpdateActivityInput) clearsDeclared(field string, current crmcontracts.Activity) bool {
	_, declared := clearableActivityColumns(current)[field]
	return declared && in.clears(field)
}
