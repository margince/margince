// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// A booked meeting with the sender does not settle a wait — only a held one
// does — but the row says so, and the caller stops counting waiting days
// against work that is already scheduled. These are the rules of
// bookedMeetingSQL, each with the case that would quietly widen or lose it.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// bookedWait seeds an unanswered inbound whose sender resolved to a contact,
// which is what the booked-meeting walk starts from.
func bookedWait(e *loadEnv, t *testing.T, contact ids.UUID) ids.UUID {
	t.Helper()
	activity := e.seedWait(t, "When can we meet?", "contact_id", contact)
	e.exec(t, `UPDATE activity_participant SET contact_id = $2
		WHERE activity_id = $1 AND role = 'from'`, activity, contact)
	return activity
}

// meetingWith seeds a meeting the contact is linked to, inserted with its own
// created_at — the column the booked rule reads a fresh booking from — so a
// test can say whether it was created before or after the message under test.
func meetingWith(
	e *loadEnv, t *testing.T, contact ids.UUID, start time.Time, status string, created time.Time,
) ids.UUID {
	t.Helper()
	meeting := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, duration_seconds, meeting_status,
			source, captured_by, created_at)
		VALUES ($1, 'meeting', 'Demo', $2, 3600, $3, 'seed', 'system', $4)`,
		meeting, start, status, created)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
		VALUES ($1, $2, 'contact', $3)`, ids.NewV7(), meeting, contact)
	return meeting
}

// The case the queue exists to stop nagging about: the customer asked, the rep
// booked, and the meeting is still ahead.
func TestABookedMeetingWithTheSenderMarksTheWait(t *testing.T) {
	e := setupLoad(t)
	contact := ids.NewV7()
	e.exec(t, `INSERT INTO contact (id, full_name, source, captured_by)
		VALUES ($1, 'Buyer Contact', 'seed', 'system')`, contact)
	activity := bookedWait(e, t, contact)
	start := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	meetingWith(e, t, contact, start, "booked", time.Now())

	got := e.waitFor(t, activity)
	if got.MeetingBookedAt == nil {
		t.Fatal("a scheduled future meeting with the sender left the wait unmarked")
	}
	if !got.MeetingBookedAt.UTC().Truncate(time.Second).Equal(start) {
		t.Fatalf("the booked moment is %v, the meeting starts %v", got.MeetingBookedAt, start)
	}
}

// The walk also reaches a meeting that names the sender as an attendee rather
// than filing under their record — the same two arms the answer predicate has.
func TestAnAttendedMeetingCountsAsBookedToo(t *testing.T) {
	e := setupLoad(t)
	contact := ids.NewV7()
	e.exec(t, `INSERT INTO contact (id, full_name, source, captured_by)
		VALUES ($1, 'Buyer Contact', 'seed', 'system')`, contact)
	activity := bookedWait(e, t, contact)
	meeting := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, duration_seconds, meeting_status,
			source, captured_by)
		VALUES ($1, 'meeting', 'Demo', now() + interval '2 days', 3600, 'booked', 'seed', 'system')`,
		meeting)
	e.exec(t, `INSERT INTO activity_participant (id, activity_id, role, contact_id)
		VALUES ($1, $2, 'attendee', $3)`, ids.NewV7(), meeting, contact)

	if got := e.waitFor(t, activity); got.MeetingBookedAt == nil {
		t.Fatal("a meeting naming the sender as attendee left the wait unmarked")
	}
}

// A meeting that predates the ask counts once the calendar MOVED it after the
// ask: the move's own audit row, the one every move writer leaves, is the
// evidence that the rep acted on the request.
func TestARescheduledMeetingCountsAsBooked(t *testing.T) {
	e := setupLoad(t)
	contact := ids.NewV7()
	e.exec(t, `INSERT INTO contact (id, full_name, source, captured_by)
		VALUES ($1, 'Buyer Contact', 'seed', 'system')`, contact)
	activity := bookedWait(e, t, contact)
	meeting := meetingWith(e, t, contact, time.Now().Add(48*time.Hour), "booked",
		time.Now().Add(-72*time.Hour))
	if got := e.waitFor(t, activity); got.MeetingBookedAt != nil {
		t.Fatal("an untouched old booking already marked the wait — the move below proves nothing now")
	}
	// A length-only edit images occurred_at unchanged on both sides, and is
	// not a move.
	e.exec(t, `INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
		VALUES ('system', 'connector:test', 'update', 'activity', $1,
		        jsonb_build_object('occurred_at', $2::timestamptz, 'duration_seconds', 3600),
		        jsonb_build_object('occurred_at', $2::timestamptz, 'duration_seconds', 1800))`,
		meeting, time.Now().Add(48*time.Hour).UTC())
	if got := e.waitFor(t, activity); got.MeetingBookedAt != nil {
		t.Fatal("a length-only edit marked the wait as booked")
	}
	e.exec(t, `INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
		VALUES ('system', 'connector:test', 'update', 'activity', $1,
		        jsonb_build_object('occurred_at', $2::timestamptz),
		        jsonb_build_object('occurred_at', $3::timestamptz))`,
		meeting, time.Now().Add(24*time.Hour).UTC(), time.Now().Add(48*time.Hour).UTC())

	if got := e.waitFor(t, activity); got.MeetingBookedAt == nil {
		t.Fatal("a meeting moved after the ask left the wait unmarked")
	}
}

// What does NOT mark a wait, one rule per case. Each seeds a meeting that
// fails exactly one predicate, so a loosened clause names itself here.
func TestWhatABookedMeetingIsNot(t *testing.T) {
	future := 48 * time.Hour
	for name, c := range map[string]struct {
		start    time.Duration
		status   string
		touched  time.Duration
		someone  string
		audience string
	}{
		"a canceled meeting":                        {start: future, status: "canceled", touched: 0},
		"a meeting already over":                    {start: -2 * time.Hour, status: "booked", touched: 0},
		"one booked before the ask and never moved": {start: future, status: "booked", touched: -72 * time.Hour},
		"somebody else's meeting":                   {start: future, status: "booked", touched: 0, someone: "other"},
		"a no-show":                                 {start: future, status: "no_show", touched: 0},
		"a participants-only meeting":               {start: future, status: "booked", touched: 0, audience: "participants"},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupLoad(t)
			contact, other := ids.NewV7(), ids.NewV7()
			for _, id := range []ids.UUID{contact, other} {
				e.exec(t, `INSERT INTO contact (id, full_name, source, captured_by)
					VALUES ($1, 'Buyer Contact', 'seed', 'system')`, id)
			}
			activity := bookedWait(e, t, contact)
			with := contact
			if c.someone != "" {
				with = other
			}
			meeting := meetingWith(e, t, with, time.Now().Add(c.start), c.status, time.Now().Add(c.touched))
			if c.audience != "" {
				e.exec(t, `UPDATE activity SET audience = $2 WHERE id = $1`, meeting, c.audience)
			}

			if got := e.waitFor(t, activity); got.MeetingBookedAt != nil {
				t.Fatalf("%s marked the wait as booked for %v", name, got.MeetingBookedAt)
			}
		})
	}
}
