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

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/momentaction"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// attentionTasks reads open tasks through the activities store. A task is an
// activity of kind `task`, so this is the same read the task queue makes.
type attentionTasks struct {
	work   *inquiryWork
	store  *activities.Store
	claims *contacts.Store
	// db reads the may-be-done question. Nil asks none.
	db *database.DB
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
	ctx context.Context, asOf, until time.Time, scope attention.TaskScope, owner ids.UUID, team []ids.UUID,
) (activities.ListActivitiesInput, bool) {
	in := activities.ListActivitiesInput{
		OpenAndDueBy: &until, IncludeEmailRequests: true, IncludeWebsiteInquiries: true, WorklistAsOf: &asOf, ReadableOnly: true, TeamQueueOf: team,
	}
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

// PlannedForViewer reads the planned lane's windows, then enriches every task
// they hold in one pass. A reminded task can also be due today; it is enriched
// once and appears in both lists. Each window's limit bounds the rows that
// qualify, because the store answers "open and due by then" itself.
func (t attentionTasks) PlannedForViewer(ctx context.Context, ask attention.PlannedAsk) (attention.PlannedTasks, error) {
	w := ask.Window
	var day, upcoming, reminders []crmcontracts.Activity
	if in, ok := openTasksDueBy(ctx, w.AsOf, w.Until, w.Scope, w.Owner, w.Team); ok {
		in.Limit = &ask.DayLimit
		var err error
		if day, err = t.listOpenTasks(ctx, in); err != nil {
			return attention.PlannedTasks{}, err
		}
	}
	if in, ok := openTasksDueBy(ctx, w.AsOf, ask.Horizon, w.Scope, w.Owner, w.Team); ok {
		in.OpenAndDueAfter, in.Limit = &w.Until, &ask.UpcomingLimit
		var err error
		if upcoming, err = t.listOpenTasks(ctx, in); err != nil {
			return attention.PlannedTasks{}, err
		}
	}
	if ask.Reminders {
		var err error
		if reminders, err = t.dueReminders(ctx, w.AsOf); err != nil {
			return attention.PlannedTasks{}, err
		}
	}
	enriched, err := t.asTasks(ctx, distinctActivities(day, upcoming, reminders), w.Scope, w.AsOf)
	if err != nil {
		return attention.PlannedTasks{}, err
	}
	byID := make(map[ids.UUID]attention.Task, len(enriched))
	for _, task := range enriched {
		byID[task.ID] = task
	}
	pick := func(rows []crmcontracts.Activity) []attention.Task {
		tasks := make([]attention.Task, 0, len(rows))
		for _, row := range rows {
			tasks = append(tasks, byID[ids.UUID(row.Id)])
		}
		return tasks
	}
	return attention.PlannedTasks{Open: pick(day), Upcoming: pick(upcoming), Reminders: pick(reminders)}, nil
}

// distinctActivities lists each activity of the windows once, in order.
func distinctActivities(windows ...[]crmcontracts.Activity) []crmcontracts.Activity {
	seen := map[openapi_types.UUID]bool{}
	var out []crmcontracts.Activity
	for _, window := range windows {
		for _, row := range window {
			if !seen[row.Id] {
				seen[row.Id] = true
				out = append(out, row)
			}
		}
	}
	return out
}

// asTasks carries a window's rows across as tasks. Each carries the reminder
// its scope may show, and a flag where the reader may not change it.
func (t attentionTasks) asTasks(ctx context.Context, rows []crmcontracts.Activity, scope attention.TaskScope, asOf time.Time) ([]attention.Task, error) {
	tasks, err := t.taskRows(ctx, rows, scope, asOf)
	if err != nil {
		return nil, err
	}
	return t.markReadOnly(ctx, tasks, asOf)
}

func (t attentionTasks) taskRows(
	ctx context.Context, rows []crmcontracts.Activity, scope attention.TaskScope, asOf time.Time,
) ([]attention.Task, error) {
	var inquiryFacts map[ids.UUID]*crmcontracts.WebsiteInquiryWork
	if t.work != nil {
		var err error
		inquiryFacts, err = t.work.ForTasks(ctx, rows)
		if err != nil {
			return nil, err
		}
	}
	tasks := make([]attention.Task, 0, len(rows))
	reader := reminderReaderFor(ctx, scope)
	for _, row := range rows {
		task := taskFromActivity(row, reader, asOf)
		task.Inquiry = inquiryFacts[ids.UUID(row.Id)]
		tasks = append(tasks, task)
	}
	return tasks, nil
}

// listOpenTasks reads one window of open tasks with each statement's generic
// plan reused (database.ReuseGenericPlans). The window spells its partial-index
// predicate as literals and caps with a literal LIMIT. The uncapped count beside
// it does not opt in: its row count follows the assignee it is bound to.
func (t attentionTasks) listOpenTasks(ctx context.Context, in activities.ListActivitiesInput) ([]crmcontracts.Activity, error) {
	var rows []crmcontracts.Activity
	err := database.ReuseGenericPlans(ctx, func(ctx context.Context) error {
		var err error
		rows, _, err = t.store.ListActivities(ctx, in)
		return err
	})
	return rows, err
}

// CountTasks counts a later task window whole, under the lane's own
// narrowing (openTasksDueBy), without the reader's due reminders.
func (t attentionTasks) CountTasks(ctx context.Context, window attention.TaskWindow) (int, error) {
	in, ok := openTasksDueBy(ctx, window.AsOf, window.Until, window.Scope, window.Owner, window.Team)
	if !ok {
		return 0, nil
	}
	in.OpenAndDueAfter = &window.From
	in.ExceptRemindersDue = window.Scope == attention.TasksMine
	return t.store.CountActivities(ctx, in)
}

// TasksAfter continues one task window past the lane's cap, in the store's
// due-date order, each task with the position after it. It reads no reminder
// arm: the lane holds every due one.
func (t attentionTasks) TasksAfter(
	ctx context.Context, window attention.TaskWindow, after string, limit int,
) ([]attention.Positioned[attention.Task], bool, error) {
	in, ok := openTasksDueBy(ctx, window.AsOf, window.Until, window.Scope, window.Owner, window.Team)
	if !ok {
		return nil, false, nil
	}
	in.Limit = &limit
	if !window.From.IsZero() {
		in.OpenAndDueAfter = &window.From
	}
	if after != "" {
		in.Cursor = &after
	}
	var rows []crmcontracts.Activity
	var positions []string
	var page storekit.Page
	err := database.ReuseGenericPlans(ctx, func(ctx context.Context) error {
		var err error
		rows, positions, page, err = t.store.ListActivitiesPositioned(ctx, in)
		return err
	})
	if err != nil {
		return nil, false, err
	}
	tasks, err := t.asTasks(ctx, rows, window.Scope, window.AsOf)
	if err != nil {
		return nil, false, err
	}
	out := make([]attention.Positioned[attention.Task], len(tasks))
	for i, task := range tasks {
		out[i] = attention.Positioned[attention.Task]{Row: task, After: positions[i]}
	}
	return out, page.HasMore, nil
}

// TasksOnDeals reads every open task of the window filed under any of these
// deals, under the lane's own narrowing (openTasksDueBy). The service judges
// which of them stand in for their deal.
func (t attentionTasks) TasksOnDeals(
	ctx context.Context, window attention.TaskWindow, deals []ids.UUID,
) ([]attention.Task, error) {
	in, ok := openTasksDueBy(ctx, window.AsOf, window.Until, window.Scope, window.Owner, window.Team)
	if !ok || len(deals) == 0 {
		return nil, nil
	}
	page := reminderPage
	in.LinkedToDeals, in.Limit = deals, &page
	var tasks []attention.Task
	for {
		rows, next, err := t.listTaskPage(ctx, in)
		if err != nil {
			return nil, err
		}
		pageTasks, err := t.taskRows(ctx, rows, window.Scope, window.AsOf)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, pageTasks...)
		if !next.HasMore {
			return tasks, nil
		}
		in.Cursor = &next.NextCursor
	}
}

// dueReminders reads every due reminder of the reader's through the store's
// reminder_due filter, the same read GET /v1/activities?reminder_due=true makes,
// following its cursor. All of them, because the badge counts all.
func (t attentionTasks) dueReminders(ctx context.Context, asOf time.Time) ([]crmcontracts.Activity, error) {
	if activities.ReminderReader(ctx).IsZero() {
		return nil, nil
	}
	var due []crmcontracts.Activity
	page := reminderPage
	in := activities.ListActivitiesInput{RemindersDue: true, AsOf: &asOf, Limit: &page, ReadableOnly: true}
	for {
		rows, next, err := t.listTaskPage(ctx, in)
		if err != nil {
			return nil, err
		}
		due = append(due, rows...)
		if !next.HasMore {
			return due, nil
		}
		in.Cursor = &next.NextCursor
	}
}

// listTaskPage reads one page of tasks reusing its generic plan, as
// listOpenTasks does, and answers where the next page starts.
func (t attentionTasks) listTaskPage(
	ctx context.Context, in activities.ListActivitiesInput,
) ([]crmcontracts.Activity, storekit.Page, error) {
	var rows []crmcontracts.Activity
	var next storekit.Page
	err := database.ReuseGenericPlans(ctx, func(ctx context.Context) error {
		var err error
		rows, next, err = t.store.ListActivities(ctx, in)
		return err
	})
	return rows, next, err
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

// markReadOnly flags the tasks the reader may see and not change. It also marks
// the request reminders Margince cannot check. Each fact is one read for the
// whole page, keyed on its ids, so the generic plans are reused
// (database.ReuseGenericPlans).
func (t attentionTasks) markReadOnly(ctx context.Context, tasks []attention.Task, asOf time.Time) ([]attention.Task, error) {
	if len(tasks) == 0 {
		return tasks, nil
	}
	named := make([]ids.UUID, len(tasks))
	for i, task := range tasks {
		named[i] = task.ID
	}
	err := database.ReuseGenericPlans(ctx, func(ctx context.Context) error {
		writable, err := t.store.WritableTasks(ctx, named)
		if err != nil {
			return err
		}
		unchecked, err := t.store.UncheckableRequestReminders(ctx, sourced(tasks), asOf)
		if err != nil {
			return err
		}
		for i := range tasks {
			tasks[i].ReadOnly = !writable[tasks[i].ID]
			inquiry := tasks[i].Inquiry
			if inquiry != nil && inquiry.CanTake != nil && *inquiry.CanTake && tasks[i].ReadOnly && (tasks[i].LinkType != entityLead || inquiry.RecordOwnerId != nil) {
				inquiry.CanTake = new(false)
			}
			if check, ok := unchecked[tasks[i].ID]; ok {
				tasks[i].RequestCheck = &check
			}
		}
		tasks, err = t.withRecipients(ctx, tasks, named)
		return err
	})
	if err != nil {
		return nil, err
	}
	return t.withMayBeDone(ctx, tasks, asOf)
}

// sourced names the tasks read out of a message, the only ones a request
// reminder can be.
func sourced(tasks []attention.Task) []ids.UUID {
	var named []ids.UUID
	for _, task := range tasks {
		if task.SourceActivityID != nil {
			named = append(named, task.ID)
		}
	}
	return named
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

// withMayBeDone asks of each promise task whether our later mail on its thread
// to a recipient kept it. The company page asks through the same seam
// (momentaction.AskTasksDone).
func (t attentionTasks) withMayBeDone(ctx context.Context, tasks []attention.Task, asOf time.Time) ([]attention.Task, error) {
	// A user or their agent answers the question, so automatic work reading the
	// queue sees the plain row.
	if answers := auth.Allows(ctx, "activity", principal.ActionRead) && auth.RequireHumanOrItsAgent(ctx) == nil; t.db == nil || !answers {
		return tasks, nil
	}
	candidates := doneCandidates(tasks)
	if len(candidates) == 0 {
		return tasks, nil
	}
	var asked map[ids.UUID]crmcontracts.WorklistMayBeDone
	err := t.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		asked, err = momentaction.AskTasksDone(ctx, tx, candidates, asOf)
		return err
	})
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return tasks, nil
	}
	if err != nil {
		return nil, err
	}
	for i := range tasks {
		if question, ok := asked[tasks[i].ID]; ok {
			tasks[i].MayBeDone = &question
		}
	}
	return tasks, nil
}

// doneCandidates are the promise tasks with a source message, each with its
// open recipients.
func doneCandidates(tasks []attention.Task) []momentaction.DoneCandidate {
	var out []momentaction.DoneCandidate
	for _, task := range tasks {
		if !task.Commitment || task.SourceActivityID == nil {
			continue
		}
		candidate := momentaction.DoneCandidate{TaskID: task.ID, SourceActivityID: *task.SourceActivityID, Assignee: task.AssigneeID}
		for _, recipient := range task.Recipients {
			if momentaction.OpenRecipient(string(recipient.Status)) {
				candidate.Recipients = append(candidate.Recipients, ids.UUID(recipient.ContactId))
			}
		}
		out = append(out, candidate)
	}
	return out
}
