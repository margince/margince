// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The task lane's reads: today's work, what is coming, and how many there are.
//
// All three share one narrowing (openTasksDueBy) so the page, the badge and the
// upcoming run cannot answer different questions about whose work this is.

package compose

import (
	"context"
	"errors"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/attention"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// attentionTasks reads open tasks through the activities store. A task is an
// activity of kind `task`, so this is the same read the task queue makes.
type attentionTasks struct {
	store  *activities.Store
	claims *contacts.Store
}

// openTasksDueBy is the ONE narrowing the lane's page and its count share.
//
// Narrowed in the QUERY, so the store's own bound applies to the rows that
// qualify. Filtering the answer instead would let a colleague's twelve tasks
// fill the page and hide the reader's own overdue one behind them — and a count
// built from a second copy of these arms would answer a different question from
// the page it sits beside, one arm at a time.
//
// asOf is what the store ages overdue tasks out of the Worklist against, so the
// page and its count drop the same ones.
//
// The false answer means "no reader to answer for", which is a page of nothing
// rather than a refusal.
func openTasksDueBy(
	ctx context.Context, asOf, until time.Time, scope attention.TaskScope, owner ids.UUID,
) (activities.ListActivitiesInput, bool) {
	in := activities.ListActivitiesInput{OpenAndDueBy: &until, IncludeEmailRequests: true, WorklistAsOf: &asOf, ReadableOnly: true}
	switch scope {
	case attention.TasksMine:
		actor, ok := principal.Actor(ctx)
		if !ok || actor.UserID.IsZero() {
			// No human, no "own work" to answer for. Reading every task and
			// calling the result theirs is the widening this narrowing exists
			// to prevent.
			return activities.ListActivitiesInput{}, false
		}
		// Exactly theirs. A task they wrote themselves carries their name from
		// the moment it is written, so this needs no unassigned arm — and the
		// arm it used to have is what put an automation's follow-up on every
		// colleague's queue.
		assignee := ids.From[ids.UserKind](actor.UserID)
		in.OwnQueueOf = &assignee
	case attention.TasksUnassigned:
		in.UnassignedQueue = true
	case attention.TasksOwnedBy:
		// One named contact's open work. The scope resolver already refused a
		// reader whose tier does not reach past themselves, and the store's own
		// row-scope gate still applies underneath — this narrows, never widens.
		named := ids.From[ids.UserKind](owner)
		in.OwnQueueOf = &named
	case attention.TasksVisible:
		// Every open task the reader may see; the row-scope gate in the store
		// is the only narrowing.
	}
	return in, true
}

func (t attentionTasks) OpenForViewer(
	ctx context.Context, asOf, until time.Time, limit int, scope attention.TaskScope, owner ids.UUID,
) ([]attention.Task, error) {
	// The store answers "open and due by then" itself, so the limit bounds the
	// rows that QUALIFY. This used to read ten times the lane and narrow
	// afterwards, which put the bound on the wrong set: a pile of completed
	// tasks filled the scan, the overdue promise underneath never reached the
	// reader, and the day rendered clear while the work was still there.
	in, ok := openTasksDueBy(ctx, asOf, until, scope, owner)
	if !ok {
		return nil, nil
	}
	in.Limit = &limit
	rows, _, err := t.store.ListActivities(ctx, in)
	if err != nil {
		return nil, err
	}
	open := make([]attention.Task, 0, len(rows))
	reader := reminderReaderFor(ctx, scope)
	for _, row := range rows {
		open = append(open, taskFromActivity(row, reader, asOf))
	}
	return t.markReadOnly(ctx, open)
}

// UpcomingForViewer reads the work due after the day's end, to a horizon.
//
// Same narrowing as OpenForViewer — the scope arms are openTasksDueBy's, so a
// colleague's tasks cannot leak into a personal lane through the newer read —
// and the window is closed at both ends in the QUERY, so the limit bounds rows
// that qualify rather than a wider set narrowed afterwards.
func (t attentionTasks) UpcomingForViewer(
	ctx context.Context, asOf, from, until time.Time, limit int, scope attention.TaskScope, owner ids.UUID,
) ([]attention.Task, error) {
	in, ok := openTasksDueBy(ctx, asOf, until, scope, owner)
	if !ok {
		return nil, nil
	}
	in.OpenAndDueAfter = &from
	in.Limit = &limit
	rows, _, err := t.store.ListActivities(ctx, in)
	if err != nil {
		return nil, err
	}
	upcoming := make([]attention.Task, 0, len(rows))
	reader := reminderReaderFor(ctx, scope)
	for _, row := range rows {
		upcoming = append(upcoming, taskFromActivity(row, reader, asOf))
	}
	return t.markReadOnly(ctx, upcoming)
}

// RemindersForViewer reads every due reminder of the reader's through the
// store's reminder_due filter, the same read GET /v1/activities?reminder_due=true
// makes, following its cursor. All of them, because the badge counts all.
func (t attentionTasks) RemindersForViewer(ctx context.Context, asOf time.Time) ([]attention.Task, error) {
	reader := activities.ReminderReader(ctx)
	if reader.IsZero() {
		return nil, nil
	}
	var due []attention.Task
	page := reminderPage
	in := activities.ListActivitiesInput{RemindersDue: true, TaskAsOf: &asOf, Limit: &page, ReadableOnly: true}
	for {
		rows, next, err := t.store.ListActivities(ctx, in)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			due = append(due, taskFromActivity(row, reader, asOf))
		}
		if !next.HasMore {
			return t.markReadOnly(ctx, due)
		}
		in.Cursor = &next.NextCursor
	}
}

// reminderPage is the store's largest page, so a reader's reminders are
// usually one read.
const reminderPage = 200

// reminderReaderFor is whose reminders a scope's rows may carry: the reader's
// on their own queue, and nobody's on any other.
func reminderReaderFor(ctx context.Context, scope attention.TaskScope) ids.UUID {
	if scope != attention.TasksMine {
		return ids.UUID{}
	}
	return activities.ReminderReader(ctx)
}

// markReadOnly flags the tasks the reader may see and not change, asked of the
// whole page in one read.
func (t attentionTasks) markReadOnly(ctx context.Context, tasks []attention.Task) ([]attention.Task, error) {
	named := make([]ids.UUID, len(tasks))
	for i, task := range tasks {
		named[i] = task.ID
	}
	writable, err := t.store.WritableTasks(ctx, named)
	if err != nil {
		return nil, err
	}
	for i := range tasks {
		tasks[i].ReadOnly = !writable[tasks[i].ID]
	}
	return t.withRecipients(ctx, tasks, named)
}

func (t attentionTasks) withRecipients(ctx context.Context, tasks []attention.Task, named []ids.UUID) ([]attention.Task, error) {
	if t.claims == nil {
		return tasks, nil
	}
	claims, err := t.claims.ClaimsOnTasks(ctx, named)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return tasks, nil
	}
	if err != nil {
		return nil, err
	}
	recipients := make(map[ids.UUID][]crmcontracts.WorklistPromiseRecipient)
	for _, claim := range claims {
		if claim.Task == nil {
			continue
		}
		recipients[*claim.Task] = append(recipients[*claim.Task], crmcontracts.WorklistPromiseRecipient{
			ClaimId: openapi_types.UUID(claim.ID), ContactId: openapi_types.UUID(claim.Contact), DisplayName: claim.DisplayName,
			Status: crmcontracts.WorklistPromiseRecipientStatus(claim.Status),
		})
	}
	for i := range tasks {
		tasks[i].Recipients = recipients[tasks[i].ID]
	}
	return tasks, nil
}
