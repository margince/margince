// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

// What a failed firing says about the rule behind it.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A FAILED FIRING NAMES THE RULE TO GO AND FIX.
//
// The entity is the RULE rather than the run, because that is where a reader
// acts — and a rule the row cannot name leaves the screen drawing "No record
// named" beside a request to fix something.
func TestATroubledRunNamesItsRule(t *testing.T) {
	t.Parallel()

	rule := ids.From[ids.AutomationKind](ids.NewV7())
	line := troubledLine(automation.TroubledAutomationRun{
		ID:           ids.NewV7(),
		AutomationID: rule,
		Name:         "File renewals under the account",
		Outcome:      "error",
		CreatedAt:    time.Date(2026, 9, 26, 9, 0, 0, 0, time.UTC),
	})
	if line.Entity == nil {
		t.Fatal("the line points at no record, so a reader has nothing to open")
	}
	if line.Entity.Type != "automation" || ids.UUID(line.Entity.Id) != rule.UUID {
		t.Errorf("entity = %+v, want the rule rather than the firing", line.Entity)
	}
	if line.Entity.Label == nil || *line.Entity.Label != "File renewals under the account" {
		t.Errorf("label = %v, want the rule's own name", line.Entity.Label)
	}
}
