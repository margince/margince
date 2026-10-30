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
	rows, err := t.listOpenTasks(ctx, in)
	if err != nil {
		return nil, err
	}
	open := make([]attention.Task, 0, len(rows))
	reader := reminderReaderFor(ctx, scope)
	for _, row := range rows {
		open = append(open, taskFromActivity(row, reader, asOf))
	}
	return t.markReadOnly(ctx, open, asOf)
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
	rows, err := t.listOpenTasks(ctx, in)
	if err != nil {
		return nil, err
	}
	upcoming := make([]attention.Task, 0, len(rows))
	reader := reminderReaderFor(ctx, scope)
	for _, row := range rows {
		upcoming = append(upcoming, taskFromActivity(row, reader, asOf))
	}
	return t.markReadOnly(ctx, upcoming, asOf)
}

// CountTasks counts one task window whole, under the lane's own narrowing
// (openTasksDueBy) and the team view's seats. Today's window counts the
// reader's due reminders with it; the later window leaves them out.
func (t attentionTasks) CountTasks(ctx context.Context, window attention.TaskWindow) (int, error) {
	in, ok := openTasksDueBy(ctx, window.AsOf, window.Until, window.Scope, window.Owner)
	if !ok {
		return 0, nil
	}
	in.TeamQueueOf = window.Team
	mine := window.Scope == attention.TasksMine
	if window.From.IsZero() {
		in.OrRemindersDue = mine
	} else {
		in.OpenAndDueAfter, in.ExceptRemindersDue = &window.From, mine
	}
	return t.store.CountActivities(ctx, in)
}

// TasksAfter continues one task window past the lane's cap, in the store's
// due-date order. It reads no reminder arm: the lane holds every due one.
// next is empty once the window is read to its end.
func (t attentionTasks) TasksAfter(
	ctx context.Context, window attention.TaskWindow, after string, limit int,
) ([]attention.Task, string, error) {
	in, ok := openTasksDueBy(ctx, window.AsOf, window.Until, window.Scope, window.Owner)
	if !ok {
		return nil, "", nil
	}
	in.TeamQueueOf, in.Limit = window.Team, &limit
	if !window.From.IsZero() {
		in.OpenAndDueAfter = &window.From
	}
	if after != "" {
		in.Cursor = &after
	}
	rows, page, err := t.listTaskPage(ctx, in)
	if err != nil {
		return nil, "", err
	}
	reader := reminderReaderFor(ctx, window.Scope)
	tasks := make([]attention.Task, 0, len(rows))
	for _, row := range rows {
		tasks = append(tasks, taskFromActivity(row, reader, window.AsOf))
	}
	tasks, err = t.markReadOnly(ctx, tasks, window.AsOf)
	if !page.HasMore {
		return tasks, "", err
	}
	return tasks, page.NextCursor, err
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
	in := activities.ListActivitiesInput{RemindersDue: true, AsOf: &asOf, Limit: &page, ReadableOnly: true}
	for {
		rows, next, err := t.listTaskPage(ctx, in)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			due = append(due, taskFromActivity(row, reader, asOf))
		}
		if !next.HasMore {
			return t.markReadOnly(ctx, due, asOf)
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

// markReadOnly flags the tasks the reader may see and not change, asked of the
// whole page in one read.
func (t attentionTasks) markReadOnly(ctx context.Context, tasks []attention.Task, asOf time.Time) ([]attention.Task, error) {
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
	tasks, err = t.withRecipients(ctx, tasks, named)
	if err != nil {
		return nil, err
	}
	return t.withMayBeDone(ctx, tasks, asOf)
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
// to a recipient kept it. It reads the follow-up rule the contact page reads
// (activities.PromiseFollowUpsTx), and the newest such mail asks. A Not yet the
// reader gave to that mail, on either surface, keeps the plain row.
func (t attentionTasks) withMayBeDone(ctx context.Context, tasks []attention.Task, asOf time.Time) ([]attention.Task, error) {
	// Only a human answers the question, so an agent reading the queue sees
	// the plain row.
	if human := auth.Allows(ctx, "activity", principal.ActionRead) && auth.RequireHuman(ctx) == nil; t.db == nil || !human {
		return tasks, nil
	}
	bases, byBasis, keys := promiseBases(tasks)
	if len(bases) == 0 {
		return tasks, nil
	}
	var followUps map[ids.UUID]activities.PromiseFollowUp
	var setAside map[momentaction.DoneSetAside]bool
	err := t.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		if followUps, err = activities.PromiseFollowUpsTx(ctx, tx, bases, asOf); err != nil {
			return err
		}
		setAside, err = momentaction.DoneSetAsidesTx(ctx, tx, keys)
		return err
	})
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return tasks, nil
	}
	if err != nil {
		return nil, err
	}
	askMayBeDone(ctx, tasks, byBasis, followUps, setAside)
	return tasks, nil
}

// promiseBasisOwner is the task and recipient one follow-up basis stands for.
type promiseBasisOwner struct{ task, contact ids.UUID }

// promiseBases builds one follow-up basis per open recipient of each promise
// task, and the question keys the reader's Not yet answers are read under.
func promiseBases(tasks []attention.Task) ([]activities.PromiseBasis, map[ids.UUID]promiseBasisOwner, []string) {
	var bases []activities.PromiseBasis
	byBasis := map[ids.UUID]promiseBasisOwner{}
	var keys []string
	for _, task := range tasks {
		if !task.Commitment || task.SourceActivityID == nil {
			continue
		}
		keys = append(keys, momentaction.DoneKey(openapi_types.UUID(task.ID)))
		for _, recipient := range task.Recipients {
			if recipient.Status != crmcontracts.WorklistPromiseRecipientStatusOpen {
				continue
			}
			basis := ids.NewV7()
			byBasis[basis] = promiseBasisOwner{task.ID, ids.UUID(recipient.ContactId)}
			bases = append(bases, activities.PromiseBasis{PromiseID: basis, SourceActivityID: *task.SourceActivityID, RecipientContactID: ids.UUID(recipient.ContactId)})
		}
	}
	return bases, byBasis, keys
}

// doneAsked is one recipient's question about a promise task.
type doneAsked struct {
	contact     ids.UUID
	sent        activities.PromiseFollowUp
	fingerprint string
}

// askMayBeDone marks each task with the question its newest unanswered
// follow-up asks.
func askMayBeDone(
	ctx context.Context, tasks []attention.Task, byBasis map[ids.UUID]promiseBasisOwner,
	followUps map[ids.UUID]activities.PromiseFollowUp, setAside map[momentaction.DoneSetAside]bool,
) {
	holders := map[ids.UUID]string{}
	for _, task := range tasks {
		holders[task.ID] = momentaction.HeldMarker(ctx, task.AssigneeID)
	}
	byTask := map[ids.UUID][]doneAsked{}
	for basis, sent := range followUps {
		owner := byBasis[basis]
		fingerprint := momentaction.DoneFingerprint(openapi_types.UUID(owner.task), sent, holders[owner.task])
		byTask[owner.task] = append(byTask[owner.task], doneAsked{contact: owner.contact, sent: sent, fingerprint: fingerprint})
	}
	for i := range tasks {
		key := momentaction.DoneKey(openapi_types.UUID(tasks[i].ID))
		found, ok := newestUnanswered(byTask[tasks[i].ID], func(a doneAsked) bool {
			return setAside[momentaction.DoneSetAside{Contact: a.contact, Key: key, Fingerprint: a.fingerprint}]
		}, func(a doneAsked) activities.PromiseFollowUp { return a.sent })
		if !ok {
			continue
		}
		tasks[i].MayBeDone = &crmcontracts.WorklistMayBeDone{
			ContactId: openapi_types.UUID(found.contact), EmailActivityId: openapi_types.UUID(found.sent.EmailID),
			WroteAt: found.sent.At, WrotePrecision: crmcontracts.OccurredPrecision(found.sent.Precision),
			ClaimKey: key, EvidenceFingerprint: found.fingerprint,
		}
	}
}

// newestUnanswered picks the follow-up a promise task asks about. That is the
// newest one, unless the reader already answered Not yet to it or a later one.
// One recipient's Not yet then holds for the task until we write to anybody
// on the thread again. Equal instants are ordered by the mail's id, as
// PromiseFollowUpsTx orders them.
func newestUnanswered[T any](candidates []T, answered func(T) bool, sent func(T) activities.PromiseFollowUp) (T, bool) {
	later := func(a, b activities.PromiseFollowUp) bool {
		if !a.At.Equal(b.At) {
			return a.At.After(b.At)
		}
		return a.EmailID.String() > b.EmailID.String()
	}
	var newest, newestAnswered T
	var have, haveAnswered bool
	for _, c := range candidates {
		if answered(c) {
			if !haveAnswered || later(sent(c), sent(newestAnswered)) {
				newestAnswered, haveAnswered = c, true
			}
			continue
		}
		if !have || later(sent(c), sent(newest)) {
			newest, have = c, true
		}
	}
	if !have || (haveAnswered && !later(sent(newest), sent(newestAnswered))) {
		var zero T
		return zero, false
	}
	return newest, true
}
