// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package momentaction

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestContactAttachRightsGateBookingAndLoggingButLeaveExistingTasksAlone(t *testing.T) {
	ctx := reader(principal.ObjectGrant{Read: true, Create: true, Update: true})
	for _, attachable := range []bool{false, true} {
		moment := crmcontracts.ContactMoment{
			RecommendedAction: crmcontracts.ContactMomentAction{
				Kind:        crmcontracts.ContactMomentActionKindScheduleMeeting,
				State:       crmcontracts.ContactMomentActionStateAvailable,
				Destination: &crmcontracts.ContactMomentDestination{Surface: crmcontracts.ContactMomentDestinationSurfaceBooking},
			},
			SecondaryActions: &[]crmcontracts.ContactMomentAction{
				{
					Kind: crmcontracts.ContactMomentActionKindLogActivity, State: crmcontracts.ContactMomentActionStateAvailable,
					Destination: &crmcontracts.ContactMomentDestination{Surface: crmcontracts.ContactMomentDestinationSurfaceActivityLog},
				},
				{
					Kind: crmcontracts.ContactMomentActionKindCompleteTask, State: crmcontracts.ContactMomentActionStateAvailable,
					Destination: &crmcontracts.ContactMomentDestination{Surface: crmcontracts.ContactMomentDestinationSurfaceTask},
				},
			},
		}
		WithholdContact(ctx, "en", &moment, attachable)
		for _, action := range []crmcontracts.ContactMomentAction{moment.RecommendedAction, (*moment.SecondaryActions)[0]} {
			if attachable {
				if action.State != crmcontracts.ContactMomentActionStateAvailable || action.Destination == nil {
					t.Fatal("an attachable contact lost its action")
				}
			} else if action.State != crmcontracts.ContactMomentActionStateBlocked || action.Destination != nil || action.BlockedReason == nil {
				t.Fatal("a read-only contact still offers to log or book")
			}
		}
		if (*moment.SecondaryActions)[1].State != crmcontracts.ContactMomentActionStateAvailable {
			t.Fatal("attach rights blocked an existing task")
		}
	}
}
