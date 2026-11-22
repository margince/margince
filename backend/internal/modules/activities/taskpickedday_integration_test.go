// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type pickedDayEnv struct {
	e     *sendEnv
	ctx   context.Context
	store *Store
}

func newPickedDayEnv(t *testing.T, companyZone string) pickedDayEnv {
	t.Helper()
	e := setupSend(t)
	setInstallationZone(t, e, companyZone)
	return pickedDayEnv{e: e, ctx: e.as(principal.RowScopeAll), store: e.store(nil)}
}

// dated files a task due on a chosen date, with no time, from a picker's zone.
func (p pickedDayEnv) dated(t *testing.T, subject, day, pickerZone string) ids.UUID {
	t.Helper()
	assignee := ids.From[ids.UserKind](p.e.rep)
	task, _, err := p.store.LogActivity(p.ctx, LogActivityInput{
		Kind: "task", Subject: new(subject), AssigneeID: &assignee,
		DueIntent: &crmcontracts.IntentTime{WallClock: day + "T23:59:59", Timezone: pickerZone},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.UUID(task.Id)
}

func (p pickedDayEnv) timed(t *testing.T, subject string, at time.Time) ids.UUID {
	t.Helper()
	assignee := ids.From[ids.UserKind](p.e.rep)
	task, _, err := p.store.LogActivity(p.ctx, LogActivityInput{
		Kind: "task", Subject: new(subject), AssigneeID: &assignee, DueAt: &at, DueHasTime: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.UUID(task.Id)
}

func (p pickedDayEnv) window(t *testing.T, in ListActivitiesInput) map[ids.UUID]bool {
	t.Helper()
	rows, _, err := p.store.ListActivities(p.ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	got := map[ids.UUID]bool{}
	for _, row := range rows {
		got[ids.UUID(row.Id)] = true
	}
	return got
}

func wantWindow(t *testing.T, name string, got map[ids.UUID]bool, want []ids.UUID, absent ...ids.UUID) {
	t.Helper()
	for _, id := range want {
		if !got[id] {
			t.Errorf("%s: a task due in this window is missing", name)
		}
	}
	for _, id := range absent {
		if got[id] {
			t.Errorf("%s: a task due outside this window is listed", name)
		}
	}
}

// Berlin company, task picked for the company's today from a zone west of it.
// Its instant lands after Berlin's midnight, but its date is today.
func TestADateOnlyTaskPickedWestOfTheCompanyIsInTodaysWindow(t *testing.T) {
	p := newPickedDayEnv(t, "Europe/Berlin")
	until := time.Date(2026, 11, 16, 23, 0, 0, 0, time.UTC)
	horizon := until.AddDate(0, 0, 7)
	asOf := time.Date(2026, 11, 16, 8, 0, 0, 0, time.UTC)

	fromLosAngeles := p.dated(t, "From Los Angeles", "2026-11-16", "America/Los_Angeles")
	fromUTC := p.dated(t, "From UTC", "2026-11-16", "UTC")
	tomorrow := p.dated(t, "Tomorrow", "2026-11-17", "UTC")
	morning := p.timed(t, "Timed", time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC))

	today := ListActivitiesInput{OpenAndDueBy: &until, WorklistAsOf: &asOf, Limit: new(4)}
	wantWindow(t, "today", p.window(t, today), []ids.UUID{fromLosAngeles, fromUTC, morning}, tomorrow)
	count, err := p.store.CountActivities(p.ctx, today)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("today's count = %d, want 3", count)
	}
	later := ListActivitiesInput{OpenAndDueBy: &horizon, OpenAndDueAfter: &until, WorklistAsOf: &asOf, Limit: new(4)}
	wantWindow(t, "upcoming", p.window(t, later), []ids.UUID{tomorrow}, fromLosAngeles, fromUTC, morning)
}

// The reverse direction: the picker is far east of a company west of UTC. The
// picked date is tomorrow for the company, though the instant is earlier than
// the company's midnight, so today's window must not take it.
func TestADateOnlyTaskPickedEastOfTheCompanyIsNotInTodaysWindow(t *testing.T) {
	p := newPickedDayEnv(t, "Etc/GMT+12")
	until := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	asOf := time.Date(2026, 11, 16, 20, 0, 0, 0, time.UTC)
	horizon := until.AddDate(0, 0, 7)

	forTomorrow := p.dated(t, "Tomorrow from Kiritimati", "2026-11-17", "Pacific/Kiritimati")
	forToday := p.dated(t, "Today from Kiritimati", "2026-11-16", "Pacific/Kiritimati")

	today := ListActivitiesInput{OpenAndDueBy: &until, WorklistAsOf: &asOf}
	wantWindow(t, "today", p.window(t, today), []ids.UUID{forToday}, forTomorrow)
	later := ListActivitiesInput{OpenAndDueBy: &horizon, OpenAndDueAfter: &until, WorklistAsOf: &asOf}
	wantWindow(t, "upcoming", p.window(t, later), []ids.UUID{forTomorrow}, forToday)
}

// A row with no chosen date keeps comparing by its instant.
func TestATaskWithNoChosenDateIsStillJudgedByItsInstant(t *testing.T) {
	p := newPickedDayEnv(t, "Europe/Berlin")
	until := time.Date(2026, 11, 16, 23, 0, 0, 0, time.UTC)
	asOf := time.Date(2026, 11, 16, 8, 0, 0, 0, time.UTC)
	before := time.Date(2026, 11, 16, 22, 0, 0, 0, time.UTC)
	after := time.Date(2026, 11, 16, 23, 30, 0, 0, time.UTC)
	assignee := ids.From[ids.UserKind](p.e.rep)
	log := func(subject string, at time.Time) ids.UUID {
		task, _, err := p.store.LogActivity(p.ctx, LogActivityInput{Kind: "task", Subject: new(subject), AssigneeID: &assignee, DueAt: &at})
		if err != nil {
			t.Fatal(err)
		}
		return ids.UUID(task.Id)
	}
	early, late := log("Before midnight", before), log("After midnight", after)
	horizon := until.AddDate(0, 0, 7)
	wantWindow(t, "today", p.window(t, ListActivitiesInput{OpenAndDueBy: &until, WorklistAsOf: &asOf}), []ids.UUID{early}, late)
	wantWindow(t, "upcoming", p.window(t,
		ListActivitiesInput{OpenAndDueBy: &horizon, OpenAndDueAfter: &until, WorklistAsOf: &asOf}), []ids.UUID{late}, early)
}

// An older client moves a task with due_at alone. The old chosen date must not
// stay behind: the task is grouped by its new due.
func TestAMoveWithDueAtAloneRegroupsADateOnlyTask(t *testing.T) {
	p := newPickedDayEnv(t, "Europe/Berlin")
	until := time.Date(2026, 11, 16, 23, 0, 0, 0, time.UTC)
	asOf := time.Date(2026, 11, 16, 8, 0, 0, 0, time.UTC)
	id := p.dated(t, "Move me", "2026-11-16", "America/Los_Angeles")
	today := ListActivitiesInput{OpenAndDueBy: &until, WorklistAsOf: &asOf}
	wantWindow(t, "before the move", p.window(t, today), []ids.UUID{id})

	moved := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	row, err := p.store.UpdateActivity(p.ctx, ids.From[ids.ActivityKind](id), UpdateActivityInput{DueAt: &moved})
	if err != nil {
		t.Fatal(err)
	}
	if row.DueIntent != nil {
		t.Fatalf("the move kept a chosen date: %+v", row.DueIntent)
	}
	wantWindow(t, "after the move", p.window(t, today), nil, id)
}

// The team board's count, the open-task sweep and Home's window share one rule.
// A date-only task is late once its date is over in the company zone.
func TestOverdueByPickedDateAgreesAcrossTheReaders(t *testing.T) {
	p := newPickedDayEnv(t, "Europe/Berlin")
	id := p.dated(t, "Yesterday from Los Angeles", "2026-11-16", "America/Los_Angeles")
	today := p.dated(t, "Today from Los Angeles", "2026-11-17", "America/Los_Angeles")
	// 08:00 in Berlin on the 17th: the instant of 16 Nov 23:59 in Los Angeles
	// (07:59 UTC) has not passed yet, yet the picked 16th is over.
	asOf := time.Date(2026, 11, 17, 7, 0, 0, 0, time.UTC)

	load, err := p.store.OverdueLoadByAssignee(p.ctx, asOf)
	if err != nil {
		t.Fatal(err)
	}
	if len(load) != 1 || load[0].Overdue != 1 {
		t.Errorf("overdue load = %+v, want one overdue task", load)
	}
	tasks, _, err := p.store.ListOpenTasks(p.ctx, ListOpenTasksInput{AsOf: asOf, MostRecentlySlippedFirst: true})
	if err != nil {
		t.Fatal(err)
	}
	late := map[ids.UUID]bool{}
	for _, task := range tasks {
		late[task.ID] = task.Overdue
	}
	if !late[id] || late[today] {
		t.Errorf("open-task sweep lateness = %v, want %s late and %s on time", late, id, today)
	}
	until := time.Date(2026, 11, 17, 23, 0, 0, 0, time.UTC)
	wantWindow(t, "today", p.window(t, ListActivitiesInput{OpenAndDueBy: &until, WorklistAsOf: &asOf}), []ids.UUID{id, today})
}

// The SQL sort key of a date-only task equals deadline.TaskDueOn on every row
// of the table both read. That covers a DST day and a final fraction of a second.
func TestTheSortKeyOfADateOnlyTaskMatchesTheGoDueRule(t *testing.T) {
	raw, err := os.ReadFile("../../shared/kernel/deadline/testdata/pickedday.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		WallClock string `json:"wall_clock"`
		Zone      string `json:"zone"`
		EndUTC    string `json:"end_utc"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, c := range cases {
		if c.EndUTC == "" {
			continue
		}
		checked++
		p := newPickedDayEnv(t, c.Zone)
		id := p.dated(t, "Sorted", c.WallClock[:10], "Asia/Tokyo")
		var got time.Time
		if err := p.e.owner.QueryRow(context.Background(),
			`SELECT `+storekit.TaskDueOrderSQL("a")+` FROM activity a WHERE a.id = $1`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		want, err := time.Parse(time.RFC3339Nano, c.EndUTC)
		if err != nil {
			t.Fatal(err)
		}
		if !got.Equal(want.Truncate(time.Microsecond)) {
			t.Errorf("%s in %s: SQL sorts it at %s, Go says %s", c.WallClock, c.Zone, got.UTC().Format(time.RFC3339Nano), c.EndUTC)
		}
	}
	if checked == 0 {
		t.Fatal("the shared table has no end_utc case, so this proves nothing")
	}
}

// A person's own list counts a chosen date in their own zone. A list that
// combines people counts it in the installation's. Berlin's day ends at 22:00
// UTC and Ho Chi Minh City's at 17:00 UTC. So one bound ends the 16th for the
// person and not for the installation.
func TestADateOnlyTaskIsJudgedInTheZoneTheListIsReadIn(t *testing.T) {
	p := newPickedDayEnv(t, "Europe/Berlin")
	until := time.Date(2026, 6, 16, 17, 0, 0, 0, time.UTC)
	asOf := time.Date(2026, 6, 15, 18, 30, 0, 0, time.UTC)

	onThe15th := p.dated(t, "Dated the 15th", "2026-06-15", "Europe/Berlin")
	onThe16th := p.dated(t, "Dated the 16th", "2026-06-16", "Europe/Berlin")
	onThe17th := p.dated(t, "Dated the 17th", "2026-06-17", "Europe/Berlin")

	own := ListActivitiesInput{OpenAndDueBy: &until, WorklistAsOf: &asOf, DayZone: "Asia/Ho_Chi_Minh"}
	wantWindow(t, "own list", p.window(t, own), []ids.UUID{onThe15th, onThe16th}, onThe17th)
	combined := ListActivitiesInput{OpenAndDueBy: &until, WorklistAsOf: &asOf}
	wantWindow(t, "combined list", p.window(t, combined), []ids.UUID{onThe15th}, onThe16th, onThe17th)
}

// The due order ends a chosen date in the same zone the overdue test counts it
// in. A task dated the 16th ends at 17:00 UTC for Ho Chi Minh City and at 22:00
// UTC for Berlin. So it sorts before a task timed 17:30 UTC on the person's own
// list, and after it on a combined one.
func TestTheDueOrderEndsAChosenDateInTheZoneTheListIsReadIn(t *testing.T) {
	p := newPickedDayEnv(t, "Europe/Berlin")
	until := time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)
	asOf := time.Date(2026, 6, 15, 18, 30, 0, 0, time.UTC)
	dated := p.dated(t, "Dated the 16th", "2026-06-16", "Europe/Berlin")
	timed := p.timed(t, "Timed 17:30 UTC on the 16th", time.Date(2026, 6, 16, 17, 30, 0, 0, time.UTC))

	order := func(zone string) []ids.UUID {
		rows, _, err := p.store.ListActivities(p.ctx, ListActivitiesInput{
			OpenAndDueBy: &until, WorklistAsOf: &asOf, DayZone: zone, Sort: new("due_at"),
		})
		if err != nil {
			t.Fatal(err)
		}
		got := make([]ids.UUID, len(rows))
		for i, row := range rows {
			got[i] = ids.UUID(row.Id)
		}
		return got
	}
	if got := order("Asia/Ho_Chi_Minh"); len(got) != 2 || got[0] != dated || got[1] != timed {
		t.Errorf("own list order = %v, want the dated task %s first, then %s", got, dated, timed)
	}
	if got := order(""); len(got) != 2 || got[0] != timed || got[1] != dated {
		t.Errorf("combined list order = %v, want the timed task %s first, then %s", got, timed, dated)
	}
}
