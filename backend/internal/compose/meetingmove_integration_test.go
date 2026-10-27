// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A meeting rescheduled in the calendar moves on the timeline.
//
// Capture wrote a meeting once, and a later pull of the same event was a replay
// that wrote nothing, so a moved meeting kept its old date. One moved more than
// a year ahead was not even delivered any more, once the calendar connector
// stopped capturing past that horizon.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func movingCalendarSink(e *integration.Env) *capture.Sink {
	return calendarSink(e).WithMeetingMover(activities.MoveCapturedMeetingFor(capture.SeatHoldsActivityTx))
}

func readMeetingStart(t *testing.T, e *integration.Env, id ids.UUID) time.Time {
	t.Helper()
	var start time.Time
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT occurred_at FROM activity WHERE id = $1`, id).Scan(&start)
	}); err != nil {
		t.Fatalf("reading the meeting start: %v", err)
	}
	return start.UTC()
}

// The same event delivered again with a new start moves the captured meeting.
func TestARescheduledEventMovesTheCapturedMeeting(t *testing.T) {
	e := integration.Setup(t)
	id := captureMeeting(t, e, e.AdminUser)
	moved := meetingStart.Add(7 * 24 * time.Hour)

	if _, err := movingCalendarSink(e).Upsert(calendarOwnerCtx(e, e.AdminUser), connector.NormalizedRecord{
		EntityType: "activity",
		NaturalKey: meetingKey,
		Fields: capture.ActivityFields{
			Kind: "meeting", Subject: "Consulting Monthly",
			Body: "Organizer: client@acme.test", OccurredAt: moved,
		},
		Source:     calendarSystem + ":" + calendarEvent,
		CapturedBy: "connector:" + calendarSystem,
		Raw:        []byte(`{"id":"` + calendarEvent + `"}`),
	}); err != nil {
		t.Fatalf("replaying the event: %v", err)
	}

	if got := readMeetingStart(t, e, id); !got.Equal(moved) {
		t.Errorf("the meeting starts %s after the calendar moved it, want %s", got, moved)
	}
}

// An event past the horizon is handed over as a move: a meeting captured
// earlier takes the new date; nothing is created for one never captured.
func TestAMoveReachesOnlyAMeetingCapturedEarlier(t *testing.T) {
	e := integration.Setup(t)
	id := captureMeeting(t, e, e.AdminUser)
	farOut := meetingStart.AddDate(2, 0, 0)
	sink := movingCalendarSink(e)
	ctx := calendarOwnerCtx(e, e.AdminUser)

	if err := sink.MoveMeeting(ctx, meetingKey, connector.MeetingSlot{Start: farOut}); err != nil {
		t.Fatalf("moving: %v", err)
	}
	if got := readMeetingStart(t, e, id); !got.Equal(farOut) {
		t.Errorf("the meeting starts %s after it was pushed out, want %s", got, farOut)
	}

	unknown := connector.NaturalKey{SourceSystem: calendarSystem, SourceID: "evt-never-captured"}
	if err := sink.MoveMeeting(ctx, unknown, connector.MeetingSlot{Start: farOut}); err != nil {
		t.Fatalf("moving an event never captured: %v", err)
	}
	var n int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM activity WHERE source_id = 'evt-never-captured'`).Scan(&n)
	}); err != nil || n != 0 {
		t.Errorf("a move created %d row(s) (err %v) for an event never captured, want none", n, err)
	}
}

// Another provider's connector may not move a calendar's meeting.
func TestAnotherConnectorCannotMoveACalendarMeeting(t *testing.T) {
	e := integration.Setup(t)
	id := captureMeeting(t, e, e.AdminUser)

	err := movingCalendarSink(e).MoveMeeting(
		connectorOwnerCtx(e, e.AdminUser, "gmail"), meetingKey, connector.MeetingSlot{Start: meetingStart.AddDate(1, 0, 0)})
	if err == nil {
		t.Error("a mail connector was allowed to move a calendar meeting")
	}
	if got := readMeetingStart(t, e, id); !got.Equal(meetingStart) {
		t.Errorf("the meeting moved to %s, want it left at %s", got, meetingStart)
	}
}

// A start outside the storable range moves nothing, as a start the calendar
// could not state moves nothing: the stored start is the better answer.
func TestAnEventMovedPastTheStorableRangeKeepsTheStoredStart(t *testing.T) {
	e := integration.Setup(t)
	id := captureMeeting(t, e, e.AdminUser)

	if _, err := movingCalendarSink(e).Upsert(calendarOwnerCtx(e, e.AdminUser), connector.NormalizedRecord{
		EntityType: "activity",
		NaturalKey: meetingKey,
		Fields: capture.ActivityFields{
			Kind: "meeting", Subject: "Consulting Monthly",
			Body: "Organizer: client@acme.test", OccurredAt: time.Date(9999, 12, 31, 23, 0, 0, 0, time.UTC),
		},
		Source:     calendarSystem + ":" + calendarEvent,
		CapturedBy: "connector:" + calendarSystem,
		Raw:        []byte(`{"id":"` + calendarEvent + `"}`),
	}); err != nil {
		t.Fatalf("replaying the event: %v", err)
	}

	if got := readMeetingStart(t, e, id); !got.Equal(meetingStart) {
		t.Errorf("the meeting starts %s, want the stored %s: no zone can render the start the calendar sent", got, meetingStart)
	}
}

// A cancellation that states a start outside the storable range still cancels.
// Its history row records no scheduled start, as for a start it could not read.
func TestACancellationPastTheStorableRangeRecordsNoScheduledStart(t *testing.T) {
	e := integration.Setup(t)
	id := captureMeeting(t, e, e.AdminUser)
	far := time.Date(9999, 12, 31, 23, 0, 0, 0, time.UTC)
	if err := calendarSink(e).CancelMeeting(calendarOwnerCtx(e, e.AdminUser), meetingKey, far); err != nil {
		t.Fatalf("cancelling the meeting: %v", err)
	}

	var startless bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT scheduled_start IS NULL FROM activity_meeting_history
			 WHERE activity_id = $1 AND status = 'canceled'`, id).Scan(&startless)
	}); err != nil {
		t.Fatalf("reading the cancellation: %v", err)
	}
	if !startless {
		t.Error("the cancellation stored a scheduled start no zone can render, want none")
	}
}

// An event that runs longer than activity.duration_seconds can hold is captured
// with no duration. Stored as sent, it fails the insert, and the calendar sync
// for that seat stops on it for good.
func TestAMeetingTooLongToStoreIsCapturedWithNoDuration(t *testing.T) {
	e := integration.Setup(t)
	decades := 3_000_000_000
	ref, err := calendarSink(e).Upsert(calendarOwnerCtx(e, e.AdminUser), connector.NormalizedRecord{
		EntityType: "activity",
		NaturalKey: meetingKey,
		Fields: capture.ActivityFields{
			Kind: "meeting", Subject: "Until 2100",
			Body: "Organizer: client@acme.test", OccurredAt: meetingStart, DurationSeconds: &decades,
		},
		Source:     calendarSystem + ":" + calendarEvent,
		CapturedBy: "connector:" + calendarSystem,
		Raw:        []byte(`{"id":"` + calendarEvent + `"}`),
	})
	if err != nil {
		t.Fatalf("capturing the meeting: %v", err)
	}

	var durationless bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT duration_seconds IS NULL FROM activity WHERE id = $1`, ref.ID).Scan(&durationless)
	}); err != nil {
		t.Fatalf("reading the meeting: %v", err)
	}
	if !durationless {
		t.Error("the meeting stored a duration, want none for one past the column's range")
	}
}

// A held meeting the calendar moves stays held only where its new start has
// taken place. Moved ahead, it is booked again, and the history says so.
func TestACalendarMoveKeepsAnOutcomeOnlyWhereTheMeetingTookPlace(t *testing.T) {
	for name, tc := range map[string]struct {
		start time.Time
		want  string
	}{
		"moved within the past": {meetingStart.Add(time.Hour), "held"},
		"moved ahead":           {time.Date(2100, 1, 4, 9, 0, 0, 0, time.UTC), "booked"},
	} {
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			id := captureMeeting(t, e, e.AdminUser)
			if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
				_, err := tx.Exec(context.Background(), `UPDATE activity SET meeting_status = 'held' WHERE id = $1`, id)
				return err
			}); err != nil {
				t.Fatalf("recording the meeting as held: %v", err)
			}

			if err := movingCalendarSink(e).MoveMeeting(calendarOwnerCtx(e, e.AdminUser), meetingKey, connector.MeetingSlot{Start: tc.start}); err != nil {
				t.Fatalf("moving: %v", err)
			}

			var status string
			var booked int
			if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
				return tx.QueryRow(context.Background(), `
					SELECT a.meeting_status, (SELECT count(*) FROM activity_meeting_history h
					        WHERE h.activity_id = a.id AND h.status = 'booked' AND h.scheduled_start = $2)
					  FROM activity a WHERE a.id = $1`, id, tc.start).Scan(&status, &booked)
			}); err != nil {
				t.Fatalf("reading the moved meeting: %v", err)
			}
			if status != tc.want {
				t.Errorf("the moved meeting reads %q, want %q", status, tc.want)
			}
			if wantBooked := map[bool]int{true: 1}[tc.want == "booked"]; booked != wantBooked {
				t.Errorf("history holds %d booked transition(s) at the new start, want %d", booked, wantBooked)
			}
		})
	}
}
