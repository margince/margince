// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// A meeting that was called off reopens the quiet reminder it answered.

import (
	"context"
	"encoding/json"
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// auditFieldIsDone is the task-done field the auto-resolve effects name, and
// the column the audit row carries it under.
const auditFieldIsDone = "is_done"

// quietReminderReopen watches a meeting's status for the two values that mean
// no contact took place.
//
// A separate handler rather than an arm of followUpAutoResolve, which watches
// activity.captured. A cancellation is an update, and the two read different
// payloads to decide whether they apply.
type quietReminderReopen struct {
	store *Store
}

// QuietReminderReopenWorkflows is the handler set, one entry, exported beside
// FollowUpWorkflows so compose registers the pair together.
func QuietReminderReopenWorkflows(store *Store) []workflow.Handler {
	return []workflow.Handler{quietReminderReopen{store: store}}
}

func (w quietReminderReopen) Spec() workflow.Spec {
	return workflow.Spec{
		Name:    "quiet_reminder_reopen_on_called_off_meeting",
		Trigger: workflow.Trigger{EventType: "activity.updated"},
		Tier:    mcp.TierAutoExecute,
		// The query sees only reminders that are done and whose record is
		// quiet. A second pass over one cancellation finds nothing left.
		RedrivableWithoutDuplicating: true,
	}
}

// Match admits only a change that set the status to canceled or no_show.
//
// Every other activity update fires this trigger, so the payload is what keeps
// a note's edit from asking the reopen question. A payload naming no meeting
// status is not a cancellation.
func (w quietReminderReopen) Match(_ context.Context, ev workflow.Event) (bool, error) {
	if len(ev.Payload) == 0 {
		return false, nil
	}
	var updated crmcontracts.PublicEventActivityUpdated
	if err := json.Unmarshal(ev.Payload, &updated); err != nil {
		return false, fmt.Errorf("activities: decoding the updated activity: %w", err)
	}
	status := updated.ChangedFields.MeetingStatus
	if status == nil {
		return false, nil
	}
	return *status == crmcontracts.MeetingWasCanceled ||
		*status == crmcontracts.MeetingWasNoShow, nil
}

// Plan states the invariant rather than the rows. The envelope carries no
// links, so which reminders reopen is Apply's query, which is the division
// followUpAutoResolve's Plan makes.
func (w quietReminderReopen) Plan(_ context.Context, ev workflow.Event) (workflow.Effect, error) {
	args, err := json.Marshal(map[string]any{auditFieldIsDone: false})
	if err != nil {
		return workflow.Effect{}, fmt.Errorf("activities: encoding the reopen effect: %w", err)
	}
	return workflow.Effect{Actions: []workflow.Action{{
		Kind: workflow.ActionUpdateRecord, Target: ev.Entity, Args: args,
	}}}, nil
}

func (w quietReminderReopen) Apply(
	ctx context.Context, ev workflow.Event, eff workflow.Effect, _ *workflow.ApprovalToken,
) (workflow.RunResult, error) {
	reopened, err := w.store.ReopenQuietRemindersStillQuietAround(
		ctx, ids.From[ids.ActivityKind](ev.Entity.ID),
	)
	if err != nil {
		return workflow.RunResult{}, fmt.Errorf(
			"reopening the quiet reminders a called-off meeting %s had answered: %w", ev.Entity.ID, err,
		)
	}
	// Nothing reopened is the ordinary answer: most called-off meetings
	// answered no reminder, and the record may have been touched since.
	if reopened == 0 {
		return workflow.RunResult{}, nil
	}
	return workflow.RunResult{Applied: eff.Actions}, nil
}

// IdempotencyKey keys on the event, the way the auto-resolve arms do: one
// delivery of one cancellation is one run, whatever the bus redelivers.
func (w quietReminderReopen) IdempotencyKey(ev workflow.Event) string {
	return "quiet_reminder_reopen:" + ev.ID.String()
}
