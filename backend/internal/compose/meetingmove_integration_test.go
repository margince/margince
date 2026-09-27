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
	return calendarSink(e).WithMeetingMover(activities.MoveCapturedMeetingTx)
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

	if err := sink.MoveMeeting(ctx, meetingKey, farOut, nil); err != nil {
		t.Fatalf("moving: %v", err)
	}
	if got := readMeetingStart(t, e, id); !got.Equal(farOut) {
		t.Errorf("the meeting starts %s after it was pushed out, want %s", got, farOut)
	}

	unknown := connector.NaturalKey{SourceSystem: calendarSystem, SourceID: "evt-never-captured"}
	if err := sink.MoveMeeting(ctx, unknown, farOut, nil); err != nil {
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
		connectorOwnerCtx(e, e.AdminUser, "gmail"), meetingKey, meetingStart.AddDate(1, 0, 0), nil)
	if err == nil {
		t.Error("a mail connector was allowed to move a calendar meeting")
	}
	if got := readMeetingStart(t, e, id); !got.Equal(meetingStart) {
		t.Errorf("the meeting moved to %s, want it left at %s", got, meetingStart)
	}
}
