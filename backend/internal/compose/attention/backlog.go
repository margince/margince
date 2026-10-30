// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The backlog behind each lane cap: what the stores count whole, so a category
// can state its exact total.

import (
	"context"
	"log/slog"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TaskWindow is one window of open tasks, read under a lane's narrowing.
type TaskWindow struct {
	AsOf time.Time
	// From is zero for today's window: due before Until, with the reader's due
	// reminders. Set, it is the later window [From, Until) without them.
	From, Until time.Time
	Scope       TaskScope
	Owner       ids.UUID
	// Team is the seats a team view keeps, plus unassigned work. Nil reads the
	// scope unnarrowed.
	Team []ids.UUID
}

// TaskBacklog counts a task window whole and continues it past the lane's
// cap. OPTIONAL: unbound, a cut task lane leaves its categories a floor and
// scrolling ends at the ranked rows.
type TaskBacklog interface {
	CountTasks(ctx context.Context, window TaskWindow) (int, error)
	// TasksAfter reads up to limit tasks of the window after a position, in
	// the store's own order. next is empty at the window's end.
	TasksAfter(ctx context.Context, window TaskWindow, after string, limit int) (tasks []Task, next string, err error)
}

// WithTaskBacklog binds the task counter, an option for the reason WithWaiting
// is one.
func (s *Service) WithTaskBacklog(b TaskBacklog) *Service {
	s.backlog.tasks = b
	return s
}

// backlogState is the backlog readers, and on one request's copy what they
// counted.
type backlogState struct {
	tasks TaskBacklog
	// exact is each source's whole count per category, for this request only.
	exact map[totalKey]int
	// until and zone are the day's end and calendar, so a task read past the
	// cap is drawn as the lane draws it.
	until time.Time
	zone  *time.Location
}

// totalKey names one source's rows in one category. A task lands in tasks
// or upcoming, so a source's count alone cannot be filed.
type totalKey struct {
	source   crmcontracts.WorklistItemSource
	category crmcontracts.WorklistCountCategory
}

// readingBacklog returns a copy carrying the stores' whole counts of every
// cut source they can count. A count that fails leaves its category a floor,
// so a failed count never fails the page.
func (s *Service) readingBacklog(
	ctx context.Context, day crmcontracts.Attention, until time.Time, zone *time.Location,
) *Service {
	reading := *s
	reading.backlog.exact = map[totalKey]int{}
	reading.backlog.until, reading.backlog.zone = until, zone
	if s.marked.probe != nil {
		return &reading
	}
	bounded := boundedSources(day)
	if bounded[sourceTask] && s.backlog.tasks != nil {
		for key, window := range s.taskWindows(day, until) {
			reading.countInto(ctx, key, window, day)
		}
	}
	return &reading
}

// taskWindows are the two task windows a page draws, by the category each
// window's rows carry: today's under tasks, the later ones under upcoming.
func (s *Service) taskWindows(day crmcontracts.Attention, until time.Time) map[totalKey]TaskWindow {
	var team []ids.UUID
	if s.teamRoster != nil {
		team = s.teamRoster.seats()
	}
	today := TaskWindow{AsOf: day.AsOf, Until: until, Scope: s.taskScope, Owner: s.taskOwner, Team: team}
	later := today
	later.From, later.Until = until, until.AddDate(0, 0, upcomingHorizonDays)
	return map[totalKey]TaskWindow{
		{sourceTask, crmcontracts.WorklistCountCategory(categoryOfSource(sourceTask))}: today,
		{sourceTask, crmcontracts.WorklistCountCategory(categoryUpcoming)}:             later,
	}
}

// countInto stores one window's count. Today's window off a non-team page is
// the planned lane's own count, which the day already carries.
func (s *Service) countInto(ctx context.Context, key totalKey, window TaskWindow, day crmcontracts.Attention) {
	if window.From.IsZero() && window.Team == nil {
		s.backlog.exact[key] = day.Counts.Planned
		return
	}
	var total int
	err := s.degradable(ctx, laneBudget, func(ctx context.Context) error {
		var err error
		total, err = s.backlog.tasks.CountTasks(ctx, window)
		return err
	})
	if err != nil {
		slog.Log(ctx, laneFailureLevel(ctx), "a worklist source could not be counted; its category stays a floor",
			"source", key.source, "category", key.category, "error", err)
		return
	}
	s.backlog.exact[key] = total
}
