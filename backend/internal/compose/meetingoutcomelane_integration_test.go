// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Which meetings still owe an answer, against a real database.
//
// The counterpart of meetinglane_integration_test.go, and it exists for the
// failure that file's own pile-of-past-meetings test describes: a bounded scan
// over `occurred_at DESC` can hide real rows behind a lane that looks plausibly
// empty. This lane is MORE exposed to it, because it reads backwards over a
// fortnight and the rows it must surface are the OLDEST — exactly the ones a
// newest-first page drops.
//
// The window itself is the other half. The lane once opened at the reader's own
// midnight, so a meeting nobody closed off left the queue when the date rolled
// over, still carrying `meeting_status` NULL. Nothing re-raised it: no other
// surface reads that column looking for work, so the record stayed wrong and
// the queue reported a clear day.

import (
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// unansweredSubjects reads the lane the way the feed does, oldest debt first.
//
// The window mirrors optionalLanes: it opens a fortnight before the day began
// and closes at the read instant. Spelled here rather than borrowed, because a
// test that took the boundary from the code under test would agree with it
// whatever it said.
func unansweredSubjects(t *testing.T, e *integration.Env, now time.Time) []string {
	t.Helper()
	lane := attentionMeetingsAwaitingOutcome{store: activities.NewStore(InstallationDB(e.Pool))}
	began := now.Truncate(24 * time.Hour)
	rows, err := lane.Since(e.Admin(), began.AddDate(0, 0, -14), now, 12, attention.TasksVisible, ids.UUID{})
	if err != nil {
		t.Fatalf("reading the outcome lane: %v", err)
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Subject)
	}
	return out
}

// YESTERDAY'S unanswered meeting is still on the lane.
//
// This is the reported bug, and the one a same-day window cannot pass: a
// meeting held yesterday morning sits before today's start, so the old boundary
// dropped it overnight while its status was still unset.
func TestAMeetingLeftUnansweredYesterdayIsStillAskedAbout(t *testing.T) {
	e := integration.Setup(t)
	now := todayAt(14 * time.Hour)
	bookMeeting(t, e, "Yesterday morning", now.AddDate(0, 0, -1).Truncate(24*time.Hour).Add(9*time.Hour), "")
	bookMeeting(t, e, "This morning", now.Truncate(24*time.Hour).Add(9*time.Hour), "")

	got := unansweredSubjects(t, e, now)
	if len(got) != 2 {
		t.Fatalf("the lane = %v, want both unanswered meetings", got)
	}
	// Oldest debt first: yesterday's has been wrong for longer.
	if got[0] != "Yesterday morning" {
		t.Errorf("the lane leads with %q, want the meeting that has waited longest", got[0])
	}
}

// A meeting from LAST WEEK is on it too, and one older than the window is not.
//
// The fortnight is a real edge rather than a figure of speech: past it the lane
// stops asking, so a reader coming back from a quiet month gets a queue they can
// clear instead of a history of everything never answered.
func TestTheOutcomeLaneReachesBackAFortnightAndNoFurther(t *testing.T) {
	e := integration.Setup(t)
	now := todayAt(14 * time.Hour)
	bookMeeting(t, e, "Last week", now.AddDate(0, 0, -7), "")
	bookMeeting(t, e, "Well outside the window", now.AddDate(0, 0, -30), "")

	got := unansweredSubjects(t, e, now)
	if len(got) != 1 || got[0] != "Last week" {
		t.Fatalf("the lane = %v, want last week's meeting and nothing older", got)
	}
}

// A settled meeting leaves the lane, whichever answer settled it.
//
// The NULL arm is the one that matters: capture writes calendar events with no
// status at all, so a predicate reading `= 'booked'` empties this lane on
// exactly the installations whose calendars are connected.
func TestAnAnsweredMeetingLeavesTheOutcomeLane(t *testing.T) {
	e := integration.Setup(t)
	now := todayAt(14 * time.Hour)
	at := now.Truncate(24 * time.Hour).Add(9 * time.Hour)
	bookMeeting(t, e, "Nobody has said", at, "")
	bookMeeting(t, e, "Still booked", at, "booked")
	bookMeeting(t, e, "Already held", at, "held")
	bookMeeting(t, e, "Called off", at, "canceled")
	bookMeeting(t, e, "Nobody came", at, "no_show")

	got := unansweredSubjects(t, e, now)
	if len(got) != 2 {
		t.Fatalf("the lane = %v, want the two meetings that still owe an answer", got)
	}
	for _, subject := range got {
		if subject != "Nobody has said" && subject != "Still booked" {
			t.Errorf("the lane carries %q, which somebody already answered", subject)
		}
	}
}

// A BACKLOG deeper than the lane's page still shows the OLDEST meetings.
//
// The failure this pins is the one the forward lane's own pile-of-past-meetings
// test describes, pointed the other way. The store orders `occurred_at DESC` and
// applies the limit in SQL, so a read asking for exactly the lane's cap gets the
// NEWEST rows in the window — while this lane renders the oldest first. Asked
// shallowly, a fortnight holding more unanswered meetings than fit would hand
// back the freshest dozen, sort those, and the meeting that has been wrong
// longest would never appear. The lane would look busy and complete while
// hiding the very rows it exists to surface.
func TestABacklogDeeperThanThePageStillLeadsWithTheOldest(t *testing.T) {
	e := integration.Setup(t)
	now := todayAt(20 * time.Hour)
	// Comfortably more unanswered meetings than the lane's cap of twelve,
	// spread across the fortnight and booked NEWEST first so the answer cannot
	// come from insertion order.
	for back := 1; back <= 40; back++ {
		at := now.Add(-time.Duration(back) * 8 * time.Hour)
		if at.Before(now.Truncate(24*time.Hour).AddDate(0, 0, -14)) {
			break
		}
		bookMeeting(t, e, fmt.Sprintf("Unanswered %02d", back), at, "")
	}

	got := unansweredSubjects(t, e, now)
	if len(got) == 0 {
		t.Fatal("the lane is empty on a fortnight full of unanswered meetings")
	}
	// The highest number is the furthest back, so it is the oldest debt and
	// must lead — whatever the cap turns out to be.
	oldest := ""
	for back := 40; back >= 1 && oldest == ""; back-- {
		at := now.Add(-time.Duration(back) * 8 * time.Hour)
		if !at.Before(now.Truncate(24*time.Hour).AddDate(0, 0, -14)) {
			oldest = fmt.Sprintf("Unanswered %02d", back)
		}
	}
	if got[0] != oldest {
		t.Errorf("the lane leads with %q, want %q — the newest-first page dropped the "+
			"oldest debt, which is the row the ordering exists to surface", got[0], oldest)
	}
}
