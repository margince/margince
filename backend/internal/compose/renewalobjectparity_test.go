// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// automation.RenewalReminderObjects and customfields.FieldObjects are two
// hand-duplicated copies of the SAME closed object vocabulary — neither
// module may import the other (ADR-0054 §9), so each spells its own list
// from the same datasource.EntityType constants rather than one deriving
// from the other. Nothing inside either module's own package can catch the
// two drifting apart; this package is where both are in scope, so this is
// where the fitness test that binds them together lives (the same posture
// agenttoolparity_test.go and agentscopeparity_test.go already take for
// their own cross-module vocabularies).

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/customfields"
)

func TestRenewalReminderObjectsMatchesFieldObjectsExactly(t *testing.T) {
	renewal := append([]string(nil), automation.RenewalReminderObjects()...)
	fields := append([]string(nil), customfields.FieldObjects...)

	// The two sets were identical until Contract, which takes custom fields
	// without being anything an automation can fire on: the record provider
	// refuses it and the preview's scope clause does not know its table. So the
	// relation is SUBSET, not equality, and every difference is named here —
	// an unnamed one is the drift this test exists to catch.
	notAutomatable := map[string]string{
		"contract": "a custom-field target that no automation can fire on: " +
			"taskeffect's owner read answers UnsupportedEntityError for it, and " +
			"automations_preview's auth.ScopeClauseFor does not know the table",
	}

	inFields := map[string]bool{}
	for _, object := range fields {
		inFields[object] = true
	}
	for _, object := range renewal {
		if !inFields[object] {
			t.Errorf("automation.RenewalReminderObjects() names %q, which is not a custom-field object — "+
				"a reminder on a date field nobody can create", object)
		}
	}
	inRenewal := map[string]bool{}
	for _, object := range renewal {
		inRenewal[object] = true
	}
	for _, object := range fields {
		if inRenewal[object] {
			continue
		}
		if _, named := notAutomatable[object]; !named {
			t.Errorf("customfields.FieldObjects names %q and automation does not, with no reason given — "+
				"either add it to RenewalReminderObjects or name it in notAutomatable with what refuses it", object)
		}
	}
	for object := range notAutomatable {
		if !inFields[object] {
			t.Errorf("notAutomatable names %q, which is not a custom-field object at all — the exception outlived its subject", object)
		}
	}
}
