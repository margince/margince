// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The meeting lanes decide eligibility and order before the database limit.
// A page cut before the status filter or in the wrong order hides the next
// valid meeting behind rows the lane throws away.

import (
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// laneDay is a fixed morning: the lanes take their window as arguments, so
// no part of these tests reads the wall clock.
var laneDay = time.Date(2026, time.March, 10, 6, 0, 0, 0, time.UTC)

func todaySubjectsAtLimit(t *testing.T, e *integration.Env, now time.Time, limit int) (subjects []string, full bool) {
	t.Helper()
	lane := attentionMeetings{store: activities.NewStore(InstallationDB(e.Pool))}
	rows, full, err := lane.Today(e.Admin(), attention.MeetingWindow{From: now, Until: now.Truncate(24 * time.Hour).Add(24 * time.Hour)}, limit, attention.TasksVisible, ids.UUID{})
	if err != nil {
		t.Fatalf("reading the meeting lane: %v", err)
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Subject)
	}
	return out, full
}

func TestCanceledMeetingsBeyondThePageDoNotHideTheNextBookedOne(t *testing.T) {
	e := integration.Setup(t)
	now := laneDay
	bookMeeting(t, e, "Next booked", now.Add(time.Hour), "booked")
	for i := 2; i <= 9; i++ {
		bookMeeting(t, e, fmt.Sprintf("Canceled %d", i), now.Add(time.Duration(i)*time.Hour), "canceled")
	}

	got, full := todaySubjectsAtLimit(t, e, now, 3)
	if len(got) != 1 || got[0] != "Next booked" {
		t.Fatalf("the lane = %v, want the booked meeting behind eight canceled ones", got)
	}
	if full {
		t.Error("the lane says it filled its limit with one open meeting")
	}
}

func TestABusyDayAtThePageLimitKeepsTheSoonestMeetings(t *testing.T) {
	e := integration.Setup(t)
	now := laneDay
	for hour := 7; hour <= 15; hour++ {
		bookMeeting(t, e, fmt.Sprintf("At %02d:00", hour), now.Truncate(24*time.Hour).Add(time.Duration(hour)*time.Hour), "")
	}

	got, full := todaySubjectsAtLimit(t, e, now, 3)
	want := []string{"At 07:00", "At 08:00", "At 09:00"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("the lane = %v, want the three soonest %v", got, want)
	}
	if !full {
		t.Error("the lane cut six meetings and says it did not fill its limit")
	}
}

func TestTheOutcomeLaneReadsTheOldestDebtAtAnyDepth(t *testing.T) {
	e := integration.Setup(t)
	now := laneDay.Add(14 * time.Hour)
	for back := 1; back <= 20; back++ {
		bookCustomerMeeting(t, e, fmt.Sprintf("Unanswered %02d", back), now.Add(-time.Duration(back)*12*time.Hour), "")
	}
	lane := attentionMeetingsAwaitingOutcome{store: activities.NewStore(InstallationDB(e.Pool))}
	rows, err := lane.Since(e.Admin(), attention.MeetingWindow{From: now.Truncate(24*time.Hour).AddDate(0, 0, -14), Until: now}, 2, attention.TasksVisible, ids.UUID{})
	if err != nil {
		t.Fatalf("reading the outcome lane: %v", err)
	}
	got := make([]string, 0, len(rows))
	for _, row := range rows {
		got = append(got, row.Subject)
	}
	want := []string{"Unanswered 20", "Unanswered 19"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("the lane = %v, want the two oldest %v", got, want)
	}
}
