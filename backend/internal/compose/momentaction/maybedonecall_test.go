// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package momentaction

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

func TestTheDoneQuestionKeepsTheCallOfAPromiseFiledFromACall(t *testing.T) {
	call := crmcontracts.ContactMomentAction{Kind: crmcontracts.ContactMomentActionKindCall, Label: "Call them", State: crmcontracts.ContactMomentActionStateAvailable}
	write := crmcontracts.ContactMomentAction{Kind: crmcontracts.ContactMomentActionKindDraftReply, Label: "Write to them", State: crmcontracts.ContactMomentActionStateWillConfirm}
	for _, tc := range []struct {
		name     string
		lead     crmcontracts.ContactMomentAction
		wantCall bool
	}{
		{"a card that led with the call", call, true},
		{"a card that led with writing", write, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plain := crmcontracts.ContactMoment{
				Rule:              crmcontracts.ContactMomentRuleOpenPromise,
				Evidence:          []crmcontracts.ContactMomentEvidence{{Type: crmcontracts.ContactMomentEvidenceTypeTask, Label: "Call Dana back"}},
				RecommendedAction: tc.lead,
			}
			sent := activities.PromiseFollowUp{EmailID: ids.NewV7(), At: time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC), Precision: "instant"}

			got := AskTaskDone(langcopy.For("en"), time.UTC, plain, ids.NewV7(), TaskAsked{Sent: sent, Key: "k", Fingerprint: "f"})

			if got.RecommendedAction.Kind != crmcontracts.ContactMomentActionKindCompleteTask {
				t.Fatalf("lead = %q, want Done", got.RecommendedAction.Kind)
			}
			hasCall := got.SecondaryActions != nil && len(*got.SecondaryActions) == 1 &&
				(*got.SecondaryActions)[0].Kind == crmcontracts.ContactMomentActionKindCall
			if hasCall != tc.wantCall {
				t.Errorf("second actions = %v, want call = %v", got.SecondaryActions, tc.wantCall)
			}
		})
	}
}
