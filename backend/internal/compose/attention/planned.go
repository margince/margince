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
	"sort"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// planned is today's agreed work, overdue first. The bound is the day's end,
// resolved once by Assemble so every due-dated lane judges the same afternoon.
func (s *Service) planned(
	ctx context.Context, asOf, until time.Time, loc *time.Location, scope TaskScope,
) ([]crmcontracts.AttentionItem, int, error) {
	open, err := s.tasks.OpenForViewer(ctx, until, plannedCap, scope, s.taskOwner)
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
		ctx, until, until.AddDate(0, 0, upcomingHorizonDays), upcomingCap, scope, s.taskOwner)
	if err != nil {
		return nil, 0, err
	}
	open = append(open, upcoming...)
	// How many there ARE, beside the page. The lane is capped at a dozen, so a
	// badge of len(items) tells a reader with thirteen that they have twelve —
	// and there is no second page on this lane to find the thirteenth by. The
	// same reading needs_you has always had.
	total, err := s.tasks.CountOpenForViewer(ctx, until, scope, s.taskOwner)
	if err != nil {
		return nil, 0, err
	}
	items := make([]crmcontracts.AttentionItem, 0, len(open))
	for _, task := range open {
		items = append(items, taskItem(task, asOf, until, loc))
	}
	// Overdue first: a promise already broken outranks one merely due, and the
	// server resolves it so every surface agrees on where the line falls.
	sort.SliceStable(items, func(i, j int) bool {
		return overdue(items[i]) && !overdue(items[j])
	})
	return items, total, nil
}
