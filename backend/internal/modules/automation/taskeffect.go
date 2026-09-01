// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

// The create_task effect, in one place.
//
// Every task-minting starter plans through here — the clock reminders in
// handlers_clock.go and the event starters in handlers_event.go alike — so the
// effect's JSON keys and the rule about who a minted task belongs to are each
// spelled once. Split out of handlers_clock.go because they were never clock
// machinery: both files call them, and only one of them owned them.

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// taskCreateEffectFor is the create_task effect shape every task-minting
// starter plans — the clock reminders in handlers_clock.go and the event
// starters in handlers_event.go alike. A task of the given subject, due at
// dueAt, linked to whatever entity fired, belonging to the given owner.
//
// Sharing the builder keeps the effect's JSON keys
// (kind/subject/due_at/assignee_id/links) in one place, so an editor-facing
// schema change lands once rather than in several hand-copied maps.
//
// A nil owner is the honest answer for a record nobody owns yet: the task is
// created unassigned and waits in the unassigned queue. It never falls back to
// whoever the workflow happened to run as, which would file the work under a
// system principal and hide it from everybody.
func taskCreateEffectFor(
	ev workflow.Event, subject string, dueAt time.Time, owner *ids.UUID,
) (workflow.Effect, error) {
	fields := map[string]any{
		fieldKind: "task",
		"subject": subject,
		"due_at":  dueAt,
		"links": []map[string]any{{
			"entity_type": string(ev.Entity.Type), "entity_id": ev.Entity.ID,
		}},
	}
	if owner != nil {
		fields["assignee_id"] = *owner
	}
	args, err := json.Marshal(fields)
	if err != nil {
		return workflow.Effect{}, fmt.Errorf("automation: encoding the task: %w", err)
	}
	return workflow.Effect{Actions: []workflow.Action{{
		Kind: workflow.ActionCreateTask, Target: ev.Entity, Args: args,
	}}}, nil
}

// anchorReminderTaskEffect is the clock handlers' view of taskCreateEffect:
// a reminder due AT the anchor moment (ev.OccurredAt), anchored on the
// fired entity, with the caller's own wording — no_activity_reminder,
// check_in_cadence, and renewal_reminder all plan through it.
func anchorReminderTaskEffect(
	ctx context.Context, ex Executors, ev workflow.Event, subject string,
) (workflow.Effect, error) {
	return ownedTaskEffect(ctx, ex, ev, subject, ev.OccurredAt)
}

// ownedTaskEffect mints a task belonging to whoever owns the record that fired.
//
// Every automation-minted task goes through here, because leaving one of them
// unowned is not a smaller version of the same bug — it is the whole bug. A task
// with no assignee reaches no rep's own queue and waits in the unassigned one
// nobody opened, so a reminder about a deal somebody owns quietly stops being
// their reminder.
//
// A record with no owner mints an unassigned task, which is honest: there is
// nobody to give it to, and the unassigned queue is where it belongs until
// somebody claims the record.
//
// A record this read cannot reach mints an unassigned task too, rather than
// failing the run. The reminder is the point; losing it entirely because its
// owner could not be looked up trades a misfiled task for no task at all.
func ownedTaskEffect(
	ctx context.Context, ex Executors, ev workflow.Event, subject string, dueAt time.Time,
) (workflow.Effect, error) {
	return taskCreateEffectFor(ev, subject, dueAt, recordOwner(ctx, ex, ev))
}

// recordOwner reads who answers for the record that fired, or nil.
//
// One shape for every record type the task-minting handlers fire on: deal,
// lead, person and organization all spell their owner `owner_id`, so one decode
// answers all four and a per-type switch would be four spellings of one fact.
func recordOwner(ctx context.Context, ex Executors, ev workflow.Event) *ids.UUID {
	if ex.Provider == nil {
		return nil
	}
	rec, err := ex.Provider.Read(ctx, ev.Entity)
	if err != nil {
		return nil
	}
	var owned struct {
		OwnerID *ids.UUID `json:"owner_id"`
	}
	if err := json.Unmarshal(rec.Fields, &owned); err != nil {
		return nil
	}
	return owned.OwnerID
}
