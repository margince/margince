// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// One follow-up is one row: an unanswered sent email and an open task derived
// from it. The task leads with what was promised; the email stays as the reason.
// The task's source activity must be the reminder's own email. The reminder is
// keyed by that email, not its thread, and a shared contact joins nothing.

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

// foldFollowUpsIntoPromises puts each follow-up reminder onto the open tasks
// derived from its email and drops the reminder. A reminder no task names
// stays as it was.
func foldFollowUpsIntoPromises(rows []ranked) []ranked {
	reminders := map[string]ranked{}
	for _, row := range rows {
		if row.item.Source == sourceAwaitingReply {
			reminders[derivedFrom(row)] = row
		}
	}
	if len(reminders) == 0 {
		return rows
	}
	folded := map[string]bool{}
	kept := make([]ranked, 0, len(rows))
	for _, row := range rows {
		email := derivedFrom(row)
		if reminder, ok := reminders[email]; ok && row.item.Source == sourceTask {
			row = withFollowUp(row, reminder)
			folded[email] = true
		}
		kept = append(kept, row)
	}
	return slices.DeleteFunc(kept, func(row ranked) bool {
		return row.item.Source == sourceAwaitingReply && folded[derivedFrom(row)]
	})
}

// withFollowUp is the promise row carrying its reminder: the email, why it
// waits, and the follow-up the composer drafts from it. The task's own verbs
// stay, so completing the row completes the task. The row is never less
// urgent than the reminder it replaces.
func withFollowUp(promise, reminder ranked) ranked {
	item := &promise.item
	item.EmailSummary = reminder.item.EmailSummary
	item.Because = slices.Concat(item.Because, reminder.item.Because)
	reply := crmcontracts.WorklistItemActionsReply
	if slices.Contains(reminder.item.Actions, reply) && !slices.Contains(item.Actions, reply) {
		item.Actions = append(slices.Clone(item.Actions), reply)
	}
	if item.Move == nil {
		item.Move = reminder.item.Move
	}
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
	if level := reminder.item.Level; level < semanticLevelOf(promise) {
		if promise.reminded {
			promise.semanticLevel = level
		} else {
			item.Level = level
		}
	}
	return promise
}

// promisePage is the store's largest page, so the tasks behind the reminders
// are usually one read.
const promisePage = 200

// readingFollowUpPromises reads, past the planned lane's cap, the open tasks
// derived from the emails the follow-up reminders are about. Without them the
// reminder and its task, met later in the tail, are two rows and two counts.
// A lane read whole already holds every such task, so it reads nothing.
func (s *Service) readingFollowUpPromises(ctx context.Context, day crmcontracts.Attention) *Service {
	scoped := *s
	todayCut, laterCut := plannedWindowsAtCap(day.Planned, true)
	if s.backlog.tasks == nil || s.backlog.until.IsZero() || !todayCut && !laterCut {
		return &scoped
	}
	sources, onTheDay := s.emailsWithoutTheirTask(day)
	if len(sources) == 0 {
		return &scoped
	}
	defer timeStage(ctx, "follow_up_promises")()
	window := TaskWindow{
		AsOf: day.AsOf, Until: s.backlog.until.AddDate(0, 0, upcomingHorizonDays),
		Scope: s.taskScope, Owner: s.taskOwner, Team: s.rosterSeats(), DerivedFrom: sources,
	}
	var tasks []Task
	err := s.degradable(ctx, "follow_up_promises", laneBudget, func(ctx context.Context) error {
		var err error
		tasks, err = s.everyTaskAfter(ctx, window)
		return err
	})
	if err != nil {
		slog.Log(ctx, laneFailureLevel(ctx), "the tasks behind follow-up reminders could not be read; each stays its own row",
			"error", err)
		return &scoped
	}
	tasks = slices.DeleteFunc(tasks, func(task Task) bool { return onTheDay[task.ID.String()] })
	zone := s.backlog.zone
	if zone == nil {
		zone = time.UTC
	}
	items := plannedItems(tasks, day.AsOf, s.backlog.until, zone)
	scoped.followUps.promises = appendLane(nil, sourceTask, &items, day.AsOf, classifyTask)
	return &scoped
}

// emailsWithoutTheirTask names the follow-up reminders' emails that no task on
// the day was derived from, and the ids of the tasks on the day.
func (s *Service) emailsWithoutTheirTask(day crmcontracts.Attention) ([]ids.UUID, map[string]bool) {
	onTheDay, tasked := map[string]bool{}, map[string]bool{}
	for _, task := range day.Planned {
		onTheDay[task.Id] = true
		if task.SourceRecord != nil {
			tasked[task.SourceRecord.Id.String()] = true
		}
	}
	var sources []ids.UUID
	for _, row := range s.followUps.rows {
		if email := derivedFrom(row); email != "" && !tasked[email] {
			sources = append(sources, ids.UUID(row.item.SourceRecord.Id))
		}
	}
	return sources, onTheDay
}

// everyTaskAfter reads the whole window through the backlog, page by page.
func (s *Service) everyTaskAfter(ctx context.Context, window TaskWindow) ([]Task, error) {
	var tasks []Task
	for after, more := "", true; more; {
		page, next, err := s.backlog.tasks.TasksAfter(ctx, window, after, promisePage)
		if err != nil {
			return nil, err
		}
		for _, task := range page {
			tasks, after = append(tasks, task.Row), task.After
		}
		more = next && len(page) > 0
	}
	return tasks, nil
}
