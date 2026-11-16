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
	"errors"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
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
	ctx context.Context, ev workflow.Event, words taskWords, record string, dueAt time.Time, owner *ids.UUID, key *taskNaturalKey,
) (workflow.Effect, error) {
	fields := map[string]any{
		fieldKind: "task",
		"subject": words.subject,
		"due_at":  dueAt,
		"links": []map[string]any{{
			"entity_type": string(ev.Entity.Type), "entity_id": ev.Entity.ID,
		}},
	}
	if owner != nil {
		fields["assignee_id"] = *owner
	}
	if key != nil {
		fields["source_system"] = key.System
		fields["source_id"] = key.ID
	}
	message := systemtext.MessageIn(ctx)
	body, why, err := words.reason(record)
	if err != nil {
		return workflow.Effect{}, err
	}
	if why != nil {
		fields["body"] = body
		message = withBody(message, why)
	}
	args, err := json.Marshal(fields)
	if err != nil {
		return workflow.Effect{}, fmt.Errorf("automation: encoding the task: %w", err)
	}
	metadata := systemtext.ForWrite(ctx, message, words.subject, body)
	return workflow.Effect{Actions: []workflow.Action{{
		Kind: workflow.ActionCreateTask, Target: ev.Entity, Args: args, SystemText: metadata,
	}}}, nil
}

// taskWords is what a minted task says: its subject, and why it exists, told
// about the record that fired it. A nil why mints a task with no body.
type taskWords struct {
	subject string
	why     func(record string) *systemtext.Component
}

// reason is the task's body, in English and as a sentence the reader's
// language renders. Without the record's name the task carries no reason.
func (w taskWords) reason(record string) (string, *systemtext.Component, error) {
	if w.why == nil || record == "" {
		return "", nil, nil
	}
	why := w.why(record)
	body, err := why.Render(textlang.English)
	if err != nil {
		return "", nil, fmt.Errorf("automation: wording the task's reason: %w", err)
	}
	return body, why, nil
}

// withBody adds the reason to the display metadata the starter already set.
func withBody(message *systemtext.Message, body *systemtext.Component) *systemtext.Message {
	out := systemtext.Message{Version: 1}
	if message != nil {
		out = *message
	}
	out.Body = body
	return &out
}

// taskNaturalKey is the identity a reminder carries so the SQL draw can see it
// (activities/lasttouch.go's openReminderHoldsEntity) and a redelivery of the
// same occurrence resolves to the row already written rather than a second one.
//
// The ID must include the anchor. replayedActivity resolves (source_system,
// source_id) WITHOUT an archived_at filter, so a key naming only the entity
// would match a reminder somebody archived months ago and the account would
// never be asked about again.
type taskNaturalKey struct{ System, ID string }

// reminderDueInDays is how far ahead a quiet-account reminder falls due.
//
// A reminder due AT its anchor is late the moment it is written: the anchor is
// the last touch, which is by definition already past the staleness threshold,
// so every task arrived overdue and the queue could not tell a real slip from
// the clock's own arithmetic. Three days is the same shape as
// defaultRouteLeadDueInDays — long enough to be actionable, short enough that
// the reminder still belongs to this week.
const reminderDueInDays = 3

// anchorReminderTaskEffect is the quiet-account handlers' view of
// taskCreateEffect: a reminder due reminderDueInDays after the firing, anchored
// on the entity that fired, carrying the handler's own occurrence key.
//
// no_activity_reminder and check_in_cadence plan through it. renewal_reminder
// does NOT: its anchor is a renewal date that can be today, so a fixed horizon
// would file the task three days after the renewal it exists to warn about.
func anchorReminderTaskEffect(
	ctx context.Context, ex Executors, ev workflow.Event, words taskWords, h workflow.Handler,
) (workflow.Effect, error) {
	key := taskNaturalKey{System: h.Spec().Name, ID: h.IdempotencyKey(ev)}
	population, err := quietPopulation(ev.Params)
	if err != nil {
		return workflow.Effect{}, err
	}
	if population.configured() {
		anchor, err := touchAnchor(ev)
		if err != nil {
			return workflow.Effect{}, err
		}
		// Reviewed quiet rules share the unique task key, including concurrent scans.
		key = taskNaturalKey{System: provenance.NoActivityReminderSource, ID: anchorIdempotencyKey(provenance.NoActivityReminderSource, ev.Entity, anchor, nil)}
	}
	return ownedTaskEffect(ctx, ex, ev, words, ev.OccurredAt.AddDate(0, 0, reminderDueInDays), &key)
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
// A deleted target skips the firing; other read failures must not create work
// under an unknown owner.
// ownedTaskEffectNoKey mints an owned task carrying no occurrence identity, for
// the starters whose effect is claimed by the runtime's own effect claim rather
// than by a natural key on the row — the event-driven follow-ups, and the
// renewal reminder whose due date stays on its anchor.
func ownedTaskEffectNoKey(
	ctx context.Context, ex Executors, ev workflow.Event, words taskWords, dueAt time.Time,
) (workflow.Effect, error) {
	return ownedTaskEffect(ctx, ex, ev, words, dueAt, nil)
}

func ownedTaskEffect(
	ctx context.Context, ex Executors, ev workflow.Event, words taskWords, dueAt time.Time, key *taskNaturalKey,
) (workflow.Effect, error) {
	if ex.Provider == nil {
		return taskCreateEffectFor(ctx, ev, words, "", dueAt, nil, key)
	}
	fired, err := firedRecord(ctx, ex, ev)
	if err != nil {
		return workflow.Effect{}, err
	}
	return taskCreateEffectFor(ctx, ev, words, fired.title(), dueAt, fired.OwnerID, key)
}

// firedFields is what a minted task needs off the record that fired: who
// answers for it, and what to call it. Deal, lead, contact and company all
// spell their owner `owner_id`, so one decode answers all four.
type firedFields struct {
	OwnerID  *ids.UUID `json:"owner_id"`
	Name     string    `json:"name"`
	FullName string    `json:"full_name"`
}

// title is the record's own name: a deal's, a company's or a contact's.
func (f firedFields) title() string {
	if f.Name != "" {
		return f.Name
	}
	return f.FullName
}

// firedRecord reads the record that fired.
func firedRecord(ctx context.Context, ex Executors, ev workflow.Event) (firedFields, error) {
	rec, err := ex.Provider.Read(ctx, ev.Entity)
	if errors.Is(err, apperrors.ErrNotFound) {
		// The event may outlive a promoted lead or an archived record.
		return firedFields{}, workflow.Declined("the target record is no longer available")
	}
	if err != nil {
		return firedFields{}, err
	}
	var fired firedFields
	if err := json.Unmarshal(rec.Fields, &fired); err != nil {
		return firedFields{}, fmt.Errorf("reading the task owner: %w", err)
	}
	return fired, nil
}
