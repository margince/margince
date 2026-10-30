// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// An attendee's own readiness for a meeting, as the Worklist, the meeting
// tool and the readiness verbs read it. The activities store computes it; this
// file maps it onto the wire once, and the agent shape is derived from that.

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// wirePreparation is the contract's MeetingPreparation for one meeting.
func wirePreparation(p activities.MeetingPreparation) crmcontracts.MeetingPreparation {
	out := crmcontracts.MeetingPreparation{
		ActivityId: openapi_types.UUID(p.ActivityID),
		State:      crmcontracts.MeetingPreparationStateNeedsReview,
		Revision:   p.Revision,
		Changed:    make([]crmcontracts.MeetingPreparationChanged, 0, len(p.Changed)),
	}
	if p.Ready {
		out.State = crmcontracts.MeetingPreparationStateReady
	}
	for _, fact := range p.Changed {
		out.Changed = append(out.Changed, crmcontracts.MeetingPreparationChanged(fact))
	}
	return out
}

// meetingReadiness binds the agent verbs and prep_for_meeting to the store.
type meetingReadiness struct{ store *activities.Store }

func (m meetingReadiness) Read(ctx context.Context, activityID ids.UUID) (agents.MeetingPreparationResult, bool, error) {
	read, err := m.store.MeetingPreparations(ctx, []ids.UUID{activityID})
	if err != nil {
		return agents.MeetingPreparationResult{}, false, err
	}
	prepared, attends := read[activityID]
	return agentPreparation(prepared), attends, nil
}

func (m meetingReadiness) Confirm(ctx context.Context, activityID ids.UUID, revision string) (agents.MeetingPreparationResult, error) {
	out, err := m.store.ConfirmMeetingReady(ctx, ids.From[ids.ActivityKind](activityID), revision)
	return agentPreparation(out), err
}

func (m meetingReadiness) Withdraw(ctx context.Context, activityID ids.UUID, revision string) (agents.MeetingPreparationResult, error) {
	out, err := m.store.WithdrawMeetingReady(ctx, ids.From[ids.ActivityKind](activityID), revision)
	return agentPreparation(out), err
}

func agentPreparation(p activities.MeetingPreparation) agents.MeetingPreparationResult {
	wire := wirePreparation(p)
	changed := make([]string, 0, len(wire.Changed))
	for _, fact := range wire.Changed {
		changed = append(changed, string(fact))
	}
	return agents.MeetingPreparationResult{
		ActivityID: ids.UUID(wire.ActivityId), State: string(wire.State), Revision: wire.Revision, Changed: changed,
	}
}
