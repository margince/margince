// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The task lane's reads: today's work, what is coming, and how many there are.
//
// All three share one narrowing (openTasksDueBy) so the page, the badge and the
// upcoming run cannot answer different questions about whose work this is.

package compose

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// attentionTasks reads open tasks through the activities store. A task is an
// activity of kind `task`, so this is the same read the task queue makes.
type attentionTasks struct{ store *activities.Store }

// openTasksDueBy is the ONE narrowing the lane's page and its count share.
//
// Narrowed in the QUERY, so the store's own bound applies to the rows that
// qualify. Filtering the answer instead would let a colleague's twelve tasks
// fill the page and hide the reader's own overdue one behind them — and a count
// built from a second copy of these arms would answer a different question from
// the page it sits beside, one arm at a time.
//
// The false answer means "no reader to answer for", which is a page of nothing
// rather than a refusal.
func openTasksDueBy(
	ctx context.Context, until time.Time, scope attention.TaskScope, owner ids.UUID,
) (activities.ListActivitiesInput, bool) {
	in := activities.ListActivitiesInput{OpenAndDueBy: &until}
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
	ctx context.Context, until time.Time, limit int, scope attention.TaskScope, owner ids.UUID,
) ([]attention.Task, error) {
	// The store answers "open and due by then" itself, so the limit bounds the
	// rows that QUALIFY. This used to read ten times the lane and narrow
	// afterwards, which put the bound on the wrong set: a pile of completed
	// tasks filled the scan, the overdue promise underneath never reached the
	// reader, and the day rendered clear while the work was still there.
	in, ok := openTasksDueBy(ctx, until, scope, owner)
	if !ok {
		return nil, nil
	}
	in.Limit = &limit
	rows, _, err := t.store.ListActivities(ctx, in)
	if err != nil {
		return nil, err
	}
	open := make([]attention.Task, 0, len(rows))
	for _, row := range rows {
		// The filter above answers only dated rows, so this skip is unreachable
		// today. It is here because the alternative to a skip is a nil deref
		// that panics the WHOLE day's page, and the guarantee lives in a WHERE
		// clause one package away — too far for the next reader of this loop to
		// see it.
		if row.DueAt == nil {
			continue
		}
		open = append(open, taskFromActivity(row))
	}
	return open, nil
}

// UpcomingForViewer reads the work due after the day's end, to a horizon.
//
// Same narrowing as OpenForViewer — the scope arms are openTasksDueBy's, so a
// colleague's tasks cannot leak into a personal lane through the newer read —
// and the window is closed at both ends in the QUERY, so the limit bounds rows
// that qualify rather than a wider set narrowed afterwards.
func (t attentionTasks) UpcomingForViewer(
	ctx context.Context, from, until time.Time, limit int, scope attention.TaskScope, owner ids.UUID,
) ([]attention.Task, error) {
	in, ok := openTasksDueBy(ctx, until, scope, owner)
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
	for _, row := range rows {
		if row.DueAt == nil {
			continue
		}
		upcoming = append(upcoming, taskFromActivity(row))
	}
	return upcoming, nil
}
