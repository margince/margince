// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// backlogTasks writes more tasks than the task lane's caps, for three seats and
// nobody: Rep1 and Rep2 share Team1, Rep3 is on Team2.
func backlogTasks(t *testing.T, r reminderEnv) {
	t.Helper()
	write := func(holder *ids.UUID, name string, count int, due func(int) time.Time) {
		for i := range count {
			r.task(t, fmt.Sprintf("%s %d", name, i), due(i), nil, holder)
		}
	}
	overdue := func(i int) time.Time { return r.now.Add(-time.Duration(i+1) * time.Hour) }
	later := func(i int) time.Time { return r.now.AddDate(0, 0, 3+i%7).Add(time.Duration(i) * time.Minute) }
	write(&r.e.Rep1, "Rep1 due", plannedCapForTest+8, overdue)
	write(&r.e.Rep1, "Rep1 later", upcomingCapForTest+2, later)
	write(&r.e.Rep2, "Rep2 due", plannedCapForTest+3, overdue)
	write(&r.e.Rep3, "Rep3 due", 5, overdue)
	write(nil, "Nobody due", 3, overdue)
	// Due in a week, reminded now: today's work on Rep1's own page only.
	remind := r.now.Add(-time.Hour)
	r.task(t, "Rep1 reminded", r.now.AddDate(0, 0, 5), &remind, &r.e.Rep1)
}

// The lane caps the tests above outgrow: attention's plannedCap and upcomingCap.
const (
	plannedCapForTest  = 12
	upcomingCapForTest = 6
)

// walkedTasks follows next_cursor to the end and answers every task row it
// was served, by category, failing on a row served twice.
func walkedTasks(
	ctx context.Context, t *testing.T, r reminderEnv, scope, filter string, owner ids.UUID,
) (map[crmcontracts.WorklistItemCategory]int, crmcontracts.Worklist) {
	t.Helper()
	svc := newAttentionService(r.e.Pool, approvals.NewService(r.e.DB()), func() time.Time { return r.now })
	seen := map[string]bool{}
	got := map[crmcontracts.WorklistItemCategory]int{}
	var first crmcontracts.Worklist
	cursor := ""
	for page := 0; ; page++ {
		if page > 50 {
			t.Fatalf("%s/%s: the walk did not end in 50 pages", scope, filter)
		}
		day, err := svc.Worklist(ctx, scope, filter, owner, 7, cursor)
		if err != nil {
			t.Fatalf("%s/%s page %d: %v", scope, filter, page, err)
		}
		if page == 0 {
			first = day
		}
		for _, row := range day.Queue {
			if row.Source != crmcontracts.WorklistItemSourceTask {
				continue
			}
			if seen[row.Id] {
				t.Fatalf("%s/%s: task %s was served twice", scope, filter, row.Id)
			}
			seen[row.Id] = true
			got[row.Category]++
		}
		if day.NextCursor == nil {
			return got, first
		}
		cursor = *day.NextCursor
	}
}

func totalOf(day crmcontracts.Worklist, category crmcontracts.WorklistCountCategory) (int, bool) {
	for _, count := range day.Counts {
		if count.Category == category && count.Total != nil {
			return *count.Total, true
		}
	}
	return 0, false
}

// Each task category's exact total is the number of task rows the list holds
// when walked to its end. That holds in every scope, and the walk reaches
// every task.
func TestTheTaskTotalsMatchTheWalkInEveryScope(t *testing.T) {
	r := setupReminders(t)
	backlogTasks(t, r)
	rep1 := r.e.As(r.e.Rep1, []ids.UUID{r.e.Team1}, integration.AdminPerms)
	checks := []struct {
		name, scope     string
		ctx             context.Context
		owner           ids.UUID
		tasks, upcoming int
		// planless: the all scope withholds the weekly plans, whose rows are
		// tasks too, so that category states no total there.
		planless bool
	}{
		// The reminded task is today's work for its holder alone.
		{"mine", "mine", rep1, ids.Nil, 21, 8, false},
		{"named owner", "mine", r.e.Admin(), r.e.Rep1, 20, 9, false},
		{"team", "team", rep1, ids.Nil, 38, 9, false},
		{"all", "all", r.e.Admin(), ids.Nil, 43, 9, true},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			walked, first := walkedTasks(check.ctx, t, r, check.scope, "", check.owner)
			total, stated := totalOf(first, "tasks")
			if stated == check.planless || (stated && total != check.tasks) || walked["tasks"] != check.tasks {
				t.Errorf("tasks: total %d (stated %v), walked %d, want %d", total, stated, walked["tasks"], check.tasks)
			}
			later, _ := walkedTasks(check.ctx, t, r, check.scope, "upcoming", check.owner)
			total, stated = totalOf(first, "upcoming")
			if !stated || total != check.upcoming || later["upcoming"] != check.upcoming {
				t.Errorf("upcoming: total %d (stated %v), walked %d, want %d", total, stated, later["upcoming"], check.upcoming)
			}
		})
	}
}
