// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Which meetings reach the day's page, against a real database.
//
// The seam reads wider than the lane and narrows in Go, so what it drops is
// decided by code the unit lane can only assert against its own stub. These
// pin it against rows the real writer produced: a held meeting is past
// preparing, a cancelled one is not happening, and a meeting tomorrow is not
// today's problem.

import (
	"fmt"
	"testing"
	"time"

	"github.com/gradionhq/margince/backend/internal/compose/integration"
	"github.com/gradionhq/margince/backend/internal/modules/activities"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// bookMeeting logs one meeting through the real activity writer at a chosen
// instant, optionally with a status.
func bookMeeting(t *testing.T, e *integration.Env, subject string, at time.Time, status string) ids.UUID {
	t.Helper()
	in := activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &at, Source: "manual",
	}
	if status != "" {
		in.MeetingStatus = &status
	}
	row, _, err := e.Activities.LogActivity(e.Admin(), in)
	if err != nil {
		t.Fatalf("booking %q: %v", subject, err)
	}
	return ids.UUID(row.Id)
}

// todayAt pins the pass's clock to a fixed hour of TODAY.
//
// The date must be real — the lane's window is a SQL predicate against the
// database's own now, so a fabricated day matches nothing. The hour must not
// be: truncating to the hour keeps the hour the suite happens to run in, so
// "still ahead today" changed meaning through the day and these tests passed
// in the morning and failed in the afternoon.
func todayAt(offset time.Duration) time.Time {
	return time.Now().UTC().Truncate(24 * time.Hour).Add(offset)
}

func meetingSubjects(t *testing.T, e *integration.Env, now time.Time) []string {
	t.Helper()
	lane := attentionMeetings{store: activities.NewStore(InstallationDB(e.Pool))}
	endOfDay := now.Truncate(24 * time.Hour).Add(24 * time.Hour)
	rows, err := lane.Today(e.Admin(), now, endOfDay, 12)
	if err != nil {
		t.Fatalf("reading the meeting lane: %v", err)
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Subject)
	}
	return out
}

// Only the meetings still ahead today, soonest first. A meeting already begun
// cannot be prepared for; one tomorrow is not today's page.
func TestTheMeetingLaneCarriesOnlyWhatIsStillAheadToday(t *testing.T) {
	e := integration.Setup(t)
	now := todayAt(9 * time.Hour)
	bookMeeting(t, e, "Started an hour ago", now.Add(-1*time.Hour), "")
	bookMeeting(t, e, "This afternoon", now.Add(4*time.Hour), "")
	bookMeeting(t, e, "In an hour", now.Add(1*time.Hour), "")
	bookMeeting(t, e, "Tomorrow morning", now.Add(26*time.Hour), "")

	got := meetingSubjects(t, e, now)
	if len(got) != 2 {
		t.Fatalf("the lane = %v, want the two meetings still ahead today", got)
	}
	if got[0] != "In an hour" || got[1] != "This afternoon" {
		t.Errorf("the lane = %v, want soonest first", got)
	}
}

// A meeting that has been held, cancelled or no-showed is past preparing. The
// lane must not ask for work that cannot be done.
func TestAFinishedOrCancelledMeetingLeavesTheLane(t *testing.T) {
	e := integration.Setup(t)
	now := todayAt(9 * time.Hour)
	bookMeeting(t, e, "Still booked", now.Add(2*time.Hour), "booked")
	bookMeeting(t, e, "Already held", now.Add(3*time.Hour), "held")
	bookMeeting(t, e, "Called off", now.Add(4*time.Hour), "canceled")
	bookMeeting(t, e, "Nobody came", now.Add(5*time.Hour), "no_show")

	got := meetingSubjects(t, e, now)
	if len(got) != 1 || got[0] != "Still booked" {
		t.Fatalf("the lane = %v, want only the meeting still worth preparing for", got)
	}
}

// A calendar event with no status is treated as booked. Capture writes them
// that way, so dropping them would empty this lane on exactly the installations
// whose calendars are connected — the failure nobody would report as a bug.
func TestAMeetingWithNoStatusIsStillWorthPreparingFor(t *testing.T) {
	e := integration.Setup(t)
	now := todayAt(9 * time.Hour)
	bookMeeting(t, e, "From the calendar", now.Add(2*time.Hour), "")

	got := meetingSubjects(t, e, now)
	if len(got) != 1 || got[0] != "From the calendar" {
		t.Fatalf("the lane = %v, want the unstatused calendar event", got)
	}
}

// A day with more LATER meetings than the lane's own bound must still show the
// soonest ones. This is the case a Go-side time filter got wrong: it read a
// fixed multiple of the lane and narrowed afterwards, so a busy calendar pushed
// the sooner meetings off the scan and the lane drew a free afternoon over a
// booked one. The window is the database's now, so the bound is the day.
func TestABusyCalendarStillShowsTheSoonestMeetings(t *testing.T) {
	e := integration.Setup(t)
	now := todayAt(6 * time.Hour)
	// Far more later-today meetings than the lane carries, booked out of order
	// so the answer cannot come from insertion order.
	for hour := 17; hour >= 8; hour-- {
		bookMeeting(t, e, fmt.Sprintf("At %02d:00", hour), now.Truncate(24*time.Hour).Add(time.Duration(hour)*time.Hour), "booked")
	}

	got := meetingSubjects(t, e, now)
	if len(got) == 0 {
		t.Fatal("the lane is empty on a day full of booked meetings")
	}
	// Whatever the bound, the FIRST one must be the soonest still ahead.
	if got[0] != "At 08:00" {
		t.Errorf("the lane leads with %q, want the soonest meeting still ahead", got[0])
	}
}

// The scan reads newest-first and the lane reads forward, so a workspace with a
// long history of past meetings must not push today's off the page.
//
// This is the shape of failure the bounded scan can hide: the lane would draw a
// free day over a day with meetings in it, and nobody would report it, because
// an empty calendar lane looks exactly like an empty calendar. Ordering makes it
// safe here — occurred_at DESC puts a future meeting above every past one — and
// this pins that rather than leaving it as a property nobody checked.
func TestAPileOfPastMeetingsDoesNotPushTodaysOffTheLane(t *testing.T) {
	e := integration.Setup(t)
	now := todayAt(9 * time.Hour)
	// Comfortably more than the lane's scan (12 * taskScanFactor).
	for day := 1; day <= 130; day++ {
		bookMeeting(t, e, fmt.Sprintf("Long ago %d", day), now.AddDate(0, 0, -day), "held")
	}
	bookMeeting(t, e, "This afternoon", now.Add(3*time.Hour), "booked")

	got := meetingSubjects(t, e, now)
	if len(got) != 1 || got[0] != "This afternoon" {
		t.Fatalf("the lane = %v, want today's meeting despite 130 older ones", got)
	}
}
