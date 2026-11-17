// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// One follow-up is one row: an unanswered sent email and the reader's own open
// task derived from it. The task leads with what was promised; the email stays
// as the reason. The task's source activity must be the reminder's own email.
// The reminder is keyed by that email, not its thread, and a shared contact
// joins nothing.

import (
	"context"
	"log/slog"
	"slices"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// derivedFrom is the activity a row was read out of: a task's source, or the
// email a follow-up reminder is about. Empty for every other row.
func derivedFrom(row ranked) string {
	record := row.item.SourceRecord
	if record == nil || record.Type != crmcontracts.AttentionSubjectTypeActivity {
		return ""
	}
	if row.item.Source != sourceTask && row.item.Source != sourceAwaitingReply {
		return ""
	}
	return record.Id.String()
}

// ownPromise says whether a task row is the reader's own open task derived
// from an email. The lane holds only live open tasks. So this is the rule
// readersPromiseSQL (modules/activities/awaitingreply.go) spells for the
// reminder's end: same email, a task, held by the reader.
func ownPromise(row ranked, reader ids.UUID) bool {
	return row.item.Source == sourceTask && !reader.IsZero() &&
		row.ownerRef == ownedBy(reader) && derivedFrom(row) != ""
}

// dueOrder is a task's place in the store's due order: undated last, then by
// id, as the task lane sorts.
type dueOrder struct {
	due time.Time
	id  string
}

func (a dueOrder) before(b dueOrder) bool {
	switch {
	case a.due.IsZero() != b.due.IsZero():
		return b.due.IsZero()
	case !a.due.Equal(b.due):
		return a.due.Before(b.due)
	}
	return a.id < b.id
}

func dueOrderOf(row ranked) dueOrder {
	return dueOrder{due: row.deadlineAt, id: row.item.Id}
}

// foldFollowUpsIntoPromises puts each follow-up reminder onto the reader's
// earliest-due promise of its email and drops the reminder. Their other
// tasks from that email stay plain rows. A reminder with no promise of the
// reader's stays as it was.
func foldFollowUpsIntoPromises(rows []ranked, reader ids.UUID) []ranked {
	reminders := map[string]ranked{}
	for _, row := range rows {
		if row.item.Source == sourceAwaitingReply {
			reminders[derivedFrom(row)] = row
		}
	}
	promises := map[string]int{}
	for i, row := range rows {
		email := derivedFrom(row)
		if _, waits := reminders[email]; !waits || !ownPromise(row, reader) {
			continue
		}
		if at, held := promises[email]; !held || dueOrderOf(row).before(dueOrderOf(rows[at])) {
			promises[email] = i
		}
	}
	if len(promises) == 0 {
		return rows
	}
	kept := make([]ranked, 0, len(rows))
	for i, row := range rows {
		email := derivedFrom(row)
		at, held := promises[email]
		switch {
		case held && i == at:
			row = withFollowUp(row, reminders[email])
		case held && row.item.Source == sourceAwaitingReply:
			continue
		}
		kept = append(kept, row)
	}
	return kept
}

// withFollowUp is the promise row carrying its reminder. That is the email,
// why it waits, and the follow-up the composer drafts from it. The task's own
// verbs stay, so completing the row completes the task. The row is never less
// urgent than the reminder it replaces. A reminder due now keeps a later
// promise in today's work, because the email is unanswered today. A snooze
// moves only the task's date.
func withFollowUp(promise, reminder ranked) ranked {
	item := &promise.item
	item.EmailSummary = reminder.item.EmailSummary
	item.Because = slices.Concat(item.Because, reminder.item.Because)
	reply := crmcontracts.WorklistItemActionsReply
	if slices.Contains(reminder.item.Actions, reply) && !slices.Contains(item.Actions, reply) {
		item.Actions = append(slices.Clone(item.Actions), reply)
	}
	item.Move = reminder.item.Move
	if item.Contact == nil {
		item.Contact = reminder.item.Contact
	}
	// The composer files the follow-up under the row's subject.
	if !openableSubject(item.Subject) {
		item.Subject = reminder.item.Subject
	}
	if promise.contact.IsZero() {
		promise.contact = reminder.contact
	}
	if item.Category == categoryUpcoming && reminder.item.Category != categoryUpcoming {
		item.Category, promise.pulledForward = reminder.item.Category, true
	}
	if level := reminder.item.Level; level < semanticLevelOf(promise) {
		if promise.reminded {
			promise.semanticLevel = level
		} else {
			item.Level = level
		}
	}
	return promise
}

// readingFollowUpPromises reads the reader's promises of the reminders' emails
// that the planned lane cut, when due before the email's promise on the day.
// A lane read whole holds every such task, so it reads nothing. Every page
// reads it: a frozen walk takes its rows from this read, and without it shows
// the reminder alone and counts the promise gone. A Done check needs it only
// for an asked reminder.
func (s *Service) readingFollowUpPromises(ctx context.Context, day crmcontracts.Attention) *Service {
	scoped := *s
	reader := readerOf(ctx)
	todayCut, laterCut := plannedWindowsAtCap(day.Planned, true)
	probedWithoutReminder := s.marked.probe != nil && len(s.marked.asked[sourceAwaitingReply]) == 0
	if s.backlog.tasks == nil || s.backlog.until.IsZero() || reader.IsZero() || !todayCut && !laterCut || probedWithoutReminder {
		return &scoped
	}
	var emails []ids.UUID
	for _, row := range s.followUps.rows {
		if row.item.Source == sourceAwaitingReply && derivedFrom(row) != "" {
			emails = append(emails, ids.UUID(row.item.SourceRecord.Id))
		}
	}
	if len(emails) == 0 {
		return &scoped
	}
	defer timeStage(ctx, "follow_up_promises")()
	var tasks []Task
	err := s.degradable(ctx, "follow_up_promises", laneBudget, func(ctx context.Context) error {
		var err error
		tasks, err = s.promisesPastTheCap(ctx, day, reader, emails)
		return err
	})
	if err != nil {
		slog.Log(ctx, laneFailureLevel(ctx), "the tasks behind follow-up reminders could not be read; each stays its own row",
			"error", err)
		return &scoped
	}
	zone := s.backlog.zone
	if zone == nil {
		zone = time.UTC
	}
	items := plannedItems(tasks, day.AsOf, s.backlog.until, zone)
	scoped.followUps.promises = appendLane(nil, sourceTask, &items, day.AsOf, classifyTask)
	return &scoped
}

// promisesPastTheCap reads the reader's promises of these emails that are not
// on the day, through the lane's own read. It skips one due after the reader's
// first promise of the same email on the day.
func (s *Service) promisesPastTheCap(ctx context.Context, day crmcontracts.Attention, reader ids.UUID, emails []ids.UUID) ([]Task, error) {
	promises, err := s.backlog.tasks.FollowUpPromises(ctx, emails)
	if err != nil {
		return nil, err
	}
	onTheDay, first := map[string]bool{}, map[string]dueOrder{}
	for _, task := range day.Planned {
		onTheDay[task.Id] = true
		record := task.SourceRecord
		if record == nil || record.Type != crmcontracts.AttentionSubjectTypeActivity ||
			task.AssigneeId == nil || ids.UUID(*task.AssigneeId) != reader {
			continue
		}
		order := dueOrder{due: deadlineOf(task.DueAt), id: task.Id}
		if held, ok := first[record.Id.String()]; !ok || order.before(held) {
			first[record.Id.String()] = order
		}
	}
	var cut []ids.UUID
	for _, promise := range promises {
		order := dueOrder{due: deadlineOf(promise.DueAt), id: promise.ID.String()}
		if held, ok := first[promise.Email.String()]; onTheDay[order.id] || ok && held.before(order) {
			continue
		}
		cut = append(cut, promise.ID)
	}
	if len(cut) == 0 {
		return nil, nil
	}
	window := TaskWindow{AsOf: day.AsOf, Until: s.backlog.until.AddDate(0, 0, upcomingHorizonDays), Scope: TasksMine}
	return s.backlog.tasks.TasksIn(ctx, window, cut)
}
