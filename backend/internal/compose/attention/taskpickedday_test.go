// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A task with a date and no time is due on the date chosen for it, in
// every reader's and company's zone.
func TestADateOnlyTaskIsGroupedByThePickedDate(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	asOf := time.Date(2026, 11, 16, 9, 0, 0, 0, berlin)
	until := storekit.StartOfNextDay(asOf, berlin)

	cases := []struct {
		name      string
		wall      string
		pickedIn  string
		want      string
		wantLate  bool
		wantToday bool
	}{
		{"picked for today from a zone west of the company", "2026-11-16T23:59:59", "America/Los_Angeles", dueGroupToday, false, true},
		{"picked for today from UTC", "2026-11-16T23:59:59", "UTC", dueGroupToday, false, true},
		{"picked for today from a zone east of the company", "2026-11-16T23:59:59", "Asia/Tokyo", dueGroupToday, false, true},
		{"picked for tomorrow from UTC", "2026-11-17T23:59:59", "UTC", dueGroupTomorrow, false, false},
		{"picked for yesterday from Tokyo", "2026-11-15T23:59:59", "Asia/Tokyo", dueGroupOverdue, true, false},
		{"a week out", "2026-11-23T23:59:59", "UTC", dueGroupThisWeek, false, false},
		{"eight days out", "2026-11-24T23:59:59", "UTC", dueGroupLater, false, false},
	}
	for _, tc := range cases {
		zone, err := time.LoadLocation(tc.pickedIn)
		if err != nil {
			t.Fatal(err)
		}
		wall, err := time.ParseInLocation("2006-01-02T15:04:05", tc.wall, zone)
		if err != nil {
			t.Fatal(err)
		}
		due := wall.UTC()
		item := taskItem(Task{
			ID: ids.NewV7(), Subject: "Send the contract", DueAt: &due,
			DueIntent: &crmcontracts.IntentTime{WallClock: tc.wall, Timezone: tc.pickedIn},
		}, asOf, until, berlin)
		if got := string(*item.DueGroup); got != tc.want {
			t.Errorf("%s: group = %q, want %q", tc.name, got, tc.want)
		}
		if *item.Overdue != tc.wantLate {
			t.Errorf("%s: overdue = %v, want %v", tc.name, *item.Overdue, tc.wantLate)
		}
		row := classifyTask(item, asOf)
		if row.overdue != tc.wantLate {
			t.Errorf("%s: ranked overdue = %v, want %v", tc.name, row.overdue, tc.wantLate)
		}
	}
}

// A task with a time keeps its instant: the company's day decides its run.
func TestATimedTaskIsStillGroupedByItsInstant(t *testing.T) {
	berlin := time.FixedZone("Europe/Berlin", 3600)
	asOf := time.Date(2026, 11, 16, 9, 0, 0, 0, berlin)
	until := storekit.StartOfNextDay(asOf, berlin)
	due := time.Date(2026, 11, 16, 23, 30, 0, 0, time.UTC)
	item := taskItem(Task{
		ID: ids.NewV7(), DueAt: &due, DueHasTime: true,
		DueIntent: &crmcontracts.IntentTime{WallClock: "2026-11-16T23:30", Timezone: "UTC"},
	}, asOf, until, berlin)
	if got := string(*item.DueGroup); got != dueGroupTomorrow {
		t.Fatalf("group = %q, want %q", got, dueGroupTomorrow)
	}
}

// The task's type reaches the queue row, where a card names the work by it.
func TestATaskRowCarriesItsType(t *testing.T) {
	asOf := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	until := storekit.StartOfNextDay(asOf, time.UTC)
	item := taskItem(Task{ID: ids.NewV7(), TaskType: new(crmcontracts.TaskTypeMeet)}, asOf, until, time.UTC)
	row := classifyTask(item, asOf).item
	if row.TaskType == nil || *row.TaskType != crmcontracts.TaskTypeMeet {
		t.Fatalf("the queue row's type is %v, want meet", row.TaskType)
	}
}
