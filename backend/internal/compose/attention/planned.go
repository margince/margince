// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The planned lane: today's agreed work, and what is coming after it.
//
// Two bounded reads rather than one, which is the whole reason this is its own
// file: the allocations must not compete, and the argument for why sits with
// the code that depends on it.

package attention

import (
	"context"
	"slices"
	"sort"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// planned is today's agreed work, overdue first. The bound is the day's end,
// resolved once by Assemble so every due-dated lane judges the same afternoon.
func (s *Service) planned(
	ctx context.Context, asOf, until time.Time, loc *time.Location, scope TaskScope,
) ([]crmcontracts.AttentionItem, int, error) {
	open, err := s.tasks.OpenForViewer(ctx, asOf, until, plannedCap, scope, s.taskOwner)
	if err != nil {
		return nil, 0, err
	}
	// What is COMING, in its own allocation. A second bounded read rather than
	// a wider one: sharing a limit would let a full day's backlog consume it
	// before a single upcoming row was reached, and the reader who most needs
	// next week's deadline is exactly the one who would never see it.
	//
	// Only when a lane is actually bound — an installation with no task reader
	// answers neither.
	upcoming, err := s.tasks.UpcomingForViewer(
		ctx, asOf, until, until.AddDate(0, 0, upcomingHorizonDays), upcomingCap, scope, s.taskOwner)
	if err != nil {
		return nil, 0, err
	}
	// Only the reader's own queue is reminded: a reminder is for its assignee,
	// and a manager looking at a colleague's work is not its assignee.
	var reminders []Task
	if scope == TasksMine {
		if reminders, err = s.tasks.RemindersForViewer(ctx, asOf, plannedCap); err != nil {
			return nil, 0, err
		}
	}
	open = remindersFirst(reminders, append(open, upcoming...))
	// How many there ARE, beside the page. The lane is capped at a dozen, so a
	// badge of len(items) tells a reader with thirteen that they have twelve —
	// and there is no second page on this lane to find the thirteenth by. The
	// same reading needs_you has always had.
	total, err := s.tasks.CountOpenForViewer(ctx, asOf, until, scope, s.taskOwner)
	if err != nil {
		return nil, 0, err
	}
	items := make([]crmcontracts.AttentionItem, 0, len(open))
	for _, task := range open {
		item := taskItem(task, asOf, until, loc)
		if task.ReminderDue {
			item.ReminderDue = new(true)
		}
		items = append(items, item)
	}
	// Due reminders first, then overdue. A promise already broken outranks one
	// merely due. The server decides both, so every surface agrees.
	sort.SliceStable(items, func(i, j int) bool {
		if reminded(items[i]) != reminded(items[j]) {
			return reminded(items[i])
		}
		return overdue(items[i]) && !overdue(items[j])
	})
	return items, total, nil
}

// remindersFirst puts the due reminders ahead of the other rows. It lists each
// task once, because a reminded task can also be due today.
func remindersFirst(reminders, rest []Task) []Task {
	seen := make(map[ids.UUID]bool, len(reminders))
	out := make([]Task, 0, len(reminders)+len(rest))
	for _, task := range slices.Concat(reminders, rest) {
		if seen[task.ID] {
			continue
		}
		seen[task.ID] = true
		out = append(out, task)
	}
	return out
}

func reminded(item crmcontracts.AttentionItem) bool {
	return item.ReminderDue != nil && *item.ReminderDue
}
