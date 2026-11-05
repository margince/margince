// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Which troubled firings offer to run again.
//
// The split is not presentation. `blocked` is the permission gate having
// refused a firing on purpose, and the retry endpoint declines it — so a row
// that offered the verb there would draw a button whose only possible outcome
// is a refusal, on the queue whose promise is that its controls do something.

import (
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestOnlyAFailedFiringOffersToRunAgain(t *testing.T) {
	for _, tc := range []struct {
		outcome string
		offers  bool
		because string
	}{
		{"failed", true, "the firing broke, which is what a retry is for"},
		{"blocked", false, "the permission gate refused it on purpose, and a retry asks the same question of the same authority"},
	} {
		t.Run(tc.outcome, func(t *testing.T) {
			item := automationItem(TroubledAutomationRun{
				ID:           ids.NewV7(),
				AutomationID: ids.New[ids.AutomationKind](),
				Name:         "Notify sales on a new lead",
				Outcome:      tc.outcome,
			})
			offered := slices.Contains(item.Actions, crmcontracts.AttentionItemActionsRetry)
			if offered != tc.offers {
				t.Fatalf("a %s firing offers retry = %v, want %v: %s",
					tc.outcome, offered, tc.offers, tc.because)
			}
		})
	}
}

// The verb rides the RUN's id, not the rule's. The endpoint re-dispatches one
// firing; `cause_ref` names the rule the firings are grouped under, and sending
// that id instead would retry nothing that exists.
func TestTheRetryVerbNamesTheFiringAndNotTheRule(t *testing.T) {
	runID := ids.NewV7()
	ruleID := ids.New[ids.AutomationKind]()
	item := automationItem(TroubledAutomationRun{
		ID: runID, AutomationID: ruleID, Name: "Notify sales", Outcome: "failed",
	})
	if item.Id != runID.String() {
		t.Fatalf("the row is identified as %q, want the run id %q — the retry endpoint "+
			"takes a run, so a row naming the rule would retry nothing", item.Id, runID)
	}
	if item.CauseRef == nil || *item.CauseRef != "automation_run:"+ruleID.String() {
		t.Fatalf("cause_ref = %v, want the RULE, which is what groups two failures of one "+
			"broken rule into one row", item.CauseRef)
	}
}
