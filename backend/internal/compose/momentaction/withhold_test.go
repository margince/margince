// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package momentaction

import (
	"context"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// reader is a rep holding exactly the activity grants named, and row scope over
// everything — so the only thing the assertions below can be answering is the
// object grant.
func reader(grant principal.ObjectGrant) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:test", UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects:  map[string]principal.ObjectGrant{"activity": grant},
			RowScope: principal.RowScopeAll,
		},
	})
}

// card carries one writing verb as the recommendation and both a writing and a
// reading verb underneath, which is the shape every real ladder produces.
func card() crmcontracts.PersonMoment {
	surface := &crmcontracts.PersonMomentDestination{
		Surface: crmcontracts.PersonMomentDestinationSurfaceActivityLog,
	}
	return crmcontracts.PersonMoment{
		RecommendedAction: crmcontracts.PersonMomentAction{
			Kind:        crmcontracts.PersonMomentActionKindLogActivity,
			Label:       "Log an interaction",
			State:       crmcontracts.PersonMomentActionStateAvailable,
			Destination: surface,
		},
		SecondaryActions: &[]crmcontracts.PersonMomentAction{
			{
				Kind:        crmcontracts.PersonMomentActionKindCompleteTask,
				Label:       "Mark it done",
				State:       crmcontracts.PersonMomentActionStateWillConfirm,
				Destination: surface,
			},
			{
				Kind:  crmcontracts.PersonMomentActionKindOpenRecord,
				Label: "Open the deal",
				State: crmcontracts.PersonMomentActionStateAvailable,
			},
		},
	}
}

func TestWithholdBlocksBothWritingVerbsForAReaderWhoCannotLog(t *testing.T) {
	moment := card()

	Withhold(reader(principal.ObjectGrant{Read: true}), &moment)

	for _, action := range append([]crmcontracts.PersonMomentAction{moment.RecommendedAction},
		(*moment.SecondaryActions)[0]) {
		if action.State != crmcontracts.PersonMomentActionStateBlocked {
			t.Errorf("%s state = %q, want blocked — its save is refused", action.Kind, action.State)
		}
		if action.BlockedReason == nil || *action.BlockedReason == "" {
			t.Errorf("%s is blocked with no reason; a control that goes quiet without saying why "+
				"is the defect this exists to prevent", action.Kind)
		}
		// The client routes on the destination, so a blocked verb that keeps
		// one is still a button that opens the form it may not save.
		if action.Destination != nil {
			t.Errorf("%s stayed blocked but kept its destination %+v", action.Kind, action.Destination)
		}
	}
}

// The reading verb beside them is the control: a blanket refusal would also
// close "Open the deal", which needs no grant this reader lacks.
func TestWithholdLeavesAReadingVerbAlone(t *testing.T) {
	moment := card()

	Withhold(reader(principal.ObjectGrant{Read: true}), &moment)

	open := (*moment.SecondaryActions)[1]
	if open.State != crmcontracts.PersonMomentActionStateAvailable {
		t.Errorf("open_record state = %q, want available — it writes nothing", open.State)
	}
}

func TestWithholdLeavesTheCardAloneForAReaderWhoMayLog(t *testing.T) {
	moment := card()

	Withhold(reader(principal.ObjectGrant{Read: true, Create: true}), &moment)

	if moment.RecommendedAction.State != crmcontracts.PersonMomentActionStateAvailable {
		t.Errorf("recommended state = %q, want available", moment.RecommendedAction.State)
	}
	if got := (*moment.SecondaryActions)[0].State; got != crmcontracts.PersonMomentActionStateWillConfirm {
		t.Errorf("complete_task state = %q, want the will_confirm it was minted with — "+
			"the withholding must not flatten a staging verb into an available one", got)
	}
}

// A card with no secondary actions is the common one (the account page mints
// exactly one verb), and the recommendation still has to be reached.
func TestWithholdBlocksTheRecommendationOnACardWithNoSecondaries(t *testing.T) {
	moment := crmcontracts.PersonMoment{
		RecommendedAction: crmcontracts.PersonMomentAction{
			Kind:  crmcontracts.PersonMomentActionKindLogActivity,
			Label: "Log something",
			State: crmcontracts.PersonMomentActionStateWillConfirm,
		},
	}

	Withhold(reader(principal.ObjectGrant{Read: true}), &moment)

	if moment.RecommendedAction.State != crmcontracts.PersonMomentActionStateBlocked {
		t.Errorf("recommended state = %q, want blocked", moment.RecommendedAction.State)
	}
}

// A verb the ladder already blocked keeps its own reason. The account card's
// "Open it from the task list" is a blocked complete_task — off because the
// task names no record beneath it — and answering it with a grant refusal would
// explain a control that was already unavailable with the wrong cause.
func TestWithholdLeavesAnAlreadyBlockedVerbSayingWhatItSaid(t *testing.T) {
	reason := "The task names no record to open"
	moment := crmcontracts.PersonMoment{
		RecommendedAction: crmcontracts.PersonMomentAction{
			Kind:          crmcontracts.PersonMomentActionKindCompleteTask,
			Label:         "Open it from the task list",
			State:         crmcontracts.PersonMomentActionStateBlocked,
			BlockedReason: &reason,
		},
	}

	Withhold(reader(principal.ObjectGrant{Read: true}), &moment)

	got := moment.RecommendedAction
	if got.BlockedReason == nil {
		t.Fatalf("blocked_reason was cleared, want the reason the ladder gave: %q", reason)
	}
	if *got.BlockedReason != reason {
		t.Errorf("blocked_reason = %q, want the reason the ladder gave: %q", *got.BlockedReason, reason)
	}
}
