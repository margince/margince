// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A cancelled or declined calendar event closes the meeting already captured.
//
// The defect this closes: a meeting reached the timeline as booked, the
// organizer called it off (or the seat declined it), and the row stayed booked
// forever. A provider stops listing an event once it is off, so the pull that
// carries the cancellation is the ONLY one that will ever mention it — dropping
// it, which is what capture did, left the meeting on the reader's schedule with
// nothing able to take it off.
//
// It lives in compose because the two halves are joined here: capture owns the
// connector door and the activities module owns `activity` and its status
// history, and neither may import the other. The seam is the subject.
//
// The meeting is seeded THROUGH THE REAL SINK rather than inserted. A
// hand-written row would prove this writer works against a shape nothing in
// production produces, and the natural key it finds the row by is exactly what
// the sink decides.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const (
	calendarSystem = "gcal"
	calendarEvent  = "evt-consulting-monthly"
)

// meetingStart is the meeting's own scheduled start — what a cancellation must
// be stamped with, rather than whenever the sync happened to notice.
var meetingStart = time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC)

// calendarSink is the Sink as compose builds it for a calendar connector: the
// cancel seam wired to the module that owns the activity table.
func calendarSink(e *integration.Env) *capture.Sink {
	return capture.NewSink(e.DB()).WithMeetingCloser(activities.CancelCapturedMeetingTx)
}

// captureMeeting lands one meeting activity through the real Sink, exactly as a
// live calendar pull would, and answers its id.
func captureMeeting(t *testing.T, e *integration.Env, owner ids.UUID) ids.UUID {
	t.Helper()
	ref, err := calendarSink(e).Upsert(calendarOwnerCtx(e, owner), connector.NormalizedRecord{
		EntityType: "activity",
		NaturalKey: connector.NaturalKey{SourceSystem: calendarSystem, SourceID: calendarEvent},
		Fields: capture.ActivityFields{
			Kind:       "meeting",
			Subject:    "Consulting Monthly",
			Body:       "Organizer: client@acme.test",
			OccurredAt: meetingStart,
		},
		Source:     calendarSystem + ":" + calendarEvent,
		CapturedBy: "connector:" + calendarSystem,
		Raw:        []byte(`{"id":"` + calendarEvent + `"}`),
	})
	if err != nil {
		t.Fatalf("capturing the meeting: %v", err)
	}
	return ref.ID
}

// calendarOwnerCtx binds the connector principal a calendar sync runs under,
// the way capture.Registry builds it: the acting connector is the calendar, and
// UserID is the seat whose calendar it is.
func calendarOwnerCtx(e *integration.Env, owner ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:" + calendarSystem,
		UserID: owner, OnBehalfOf: owner,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"activity":     {Create: true, Read: true, Update: true},
				"person":       {Create: true, Read: true, Update: true},
				"organization": {Create: true, Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

// meetingKey is the natural key the capture landed under — what a later pull
// finds the row by.
var meetingKey = connector.NaturalKey{SourceSystem: calendarSystem, SourceID: calendarEvent}

// readMeetingStatus answers the stored status, and whether it is set at all.
func readMeetingStatus(t *testing.T, e *integration.Env, id ids.UUID) (string, bool) {
	t.Helper()
	var status *string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT meeting_status FROM activity WHERE id = $1`, id).Scan(&status)
	}); err != nil {
		t.Fatalf("reading the meeting status: %v", err)
	}
	if status == nil {
		return "", false
	}
	return *status, true
}

// The defect, end to end: a meeting captured while it was live is closed when
// the calendar says it is off.
func TestACancellationClosesTheCapturedMeeting(t *testing.T) {
	e := integration.Setup(t)
	id := captureMeeting(t, e, e.AdminUser)

	// A meeting arrives from a calendar with NO status — the state every one of
	// these rows is in until somebody answers for it.
	if status, set := readMeetingStatus(t, e, id); set {
		t.Fatalf("the captured meeting arrived as %q, want no status at all", status)
	}

	if err := calendarSink(e).CancelMeeting(calendarOwnerCtx(e, e.AdminUser), meetingKey, meetingStart); err != nil {
		t.Fatalf("cancelling the meeting: %v", err)
	}

	status, set := readMeetingStatus(t, e, id)
	if !set || status != "canceled" {
		t.Fatalf("meeting_status = %q (set=%v), want canceled — the meeting stays on the schedule otherwise", status, set)
	}
}

// The transition, which is what every question about a PERIOD reads. The column
// alone answers "what is this meeting now" and cannot answer "how many fell
// through last week".
func TestACancellationRecordsItsTransition(t *testing.T) {
	e := integration.Setup(t)
	id := captureMeeting(t, e, e.AdminUser)

	if err := calendarSink(e).CancelMeeting(calendarOwnerCtx(e, e.AdminUser), meetingKey, meetingStart); err != nil {
		t.Fatalf("cancelling the meeting: %v", err)
	}

	var status string
	var scheduledStart time.Time
	var sourceSystem, sourceID *string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT status, scheduled_start, source_system, source_id
			  FROM activity_meeting_history WHERE activity_id = $1`, id).
			Scan(&status, &scheduledStart, &sourceSystem, &sourceID)
	}); err != nil {
		t.Fatalf("reading the transition: %v", err)
	}
	if status != "canceled" {
		t.Errorf("transition status = %q, want canceled", status)
	}
	// The meeting's OWN start, not the moment the sync noticed. A pull running
	// days later would otherwise date the cancellation to the sync schedule, so
	// every reading of when bookings fall through would follow the cron.
	if !scheduledStart.Equal(meetingStart) {
		t.Errorf("scheduled_start = %v, want the meeting's start %v", scheduledStart, meetingStart)
	}
	// The idempotency key, which is what makes a resync free.
	if sourceSystem == nil || *sourceSystem != calendarSystem || sourceID == nil || *sourceID != calendarEvent {
		t.Errorf("transition source = (%v, %v), want the calendar's own key", sourceSystem, sourceID)
	}
}

// A calendar resyncs constantly and a cancelled event keeps arriving. The second
// pull must write nothing: no second audit row, no second event on the bus, no
// second transition doubling every cancellation count.
func TestCancellingTwiceWritesOnce(t *testing.T) {
	e := integration.Setup(t)
	id := captureMeeting(t, e, e.AdminUser)

	for i := range 3 {
		if err := calendarSink(e).CancelMeeting(calendarOwnerCtx(e, e.AdminUser), meetingKey, meetingStart); err != nil {
			t.Fatalf("cancelling the meeting (pass %d): %v", i+1, err)
		}
	}

	var transitions, updates int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM activity_meeting_history WHERE activity_id = $1`, id).
			Scan(&transitions); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM audit_log
			 WHERE entity_type = 'activity' AND entity_id = $1 AND action = 'update'`, id).
			Scan(&updates)
	}); err != nil {
		t.Fatalf("counting what three cancellations wrote: %v", err)
	}
	if transitions != 1 {
		t.Errorf("three cancellations wrote %d transitions, want 1 — every resync would double the count", transitions)
	}
	if updates != 1 {
		t.Errorf("three cancellations wrote %d audit rows, want 1", updates)
	}
}

// A key naming no captured meeting is the ORDINARY case: most cancelled events
// were never worth capturing at all — an internal meeting, a solo block. It must
// not fail, or one such event would stop a whole calendar sync.
func TestCancellingAMeetingNobodyCapturedIsNotAFault(t *testing.T) {
	e := integration.Setup(t)
	unknown := connector.NaturalKey{SourceSystem: calendarSystem, SourceID: "evt-never-captured"}
	if err := calendarSink(e).CancelMeeting(calendarOwnerCtx(e, e.AdminUser), unknown, meetingStart); err != nil {
		t.Fatalf("cancelling an uncaptured meeting: %v — a sync would stop on every internal meeting", err)
	}
}

// An ARCHIVED meeting is left where somebody put it. A calendar sync is not the
// caller entitled to move the status of a row a person deliberately retired.
func TestAnArchivedMeetingIsNotReopenedByASync(t *testing.T) {
	e := integration.Setup(t)
	id := captureMeeting(t, e, e.AdminUser)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE activity SET archived_at = now() WHERE id = $1`, id)
		return err
	}); err != nil {
		t.Fatalf("archiving the meeting: %v", err)
	}

	if err := calendarSink(e).CancelMeeting(calendarOwnerCtx(e, e.AdminUser), meetingKey, meetingStart); err != nil {
		t.Fatalf("cancelling an archived meeting: %v — a sync must survive one", err)
	}
	if status, set := readMeetingStatus(t, e, id); set {
		t.Errorf("meeting_status = %q on an archived row, want it left alone", status)
	}
}

// A meeting somebody ANSWERED FOR keeps their answer.
//
// `held` and `no_show` are recorded by a person who knows what happened. A
// calendar disagreeing afterwards — the organizer tidying up a past event, a
// late deletion — does not un-happen a meeting that took place, and overwriting
// the answer would delete the only record that it did: silently, on a sync
// schedule nobody is watching, and against the person who took the trouble to
// record it.
func TestAnAnsweredMeetingKeepsItsOutcome(t *testing.T) {
	e := integration.Setup(t)
	for _, answered := range []string{"held", "no_show"} {
		t.Run(answered, func(t *testing.T) {
			id := captureMeeting(t, e, e.AdminUser)
			t.Cleanup(func() {
				// The natural key is one per workspace, so each arm clears the
				// row it captured rather than colliding with the next.
				if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
					_, err := tx.Exec(context.Background(), `DELETE FROM activity WHERE id = $1`, id)
					return err
				}); err != nil {
					t.Fatalf("clearing the seeded meeting: %v", err)
				}
			})
			if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
				_, err := tx.Exec(context.Background(),
					`UPDATE activity SET meeting_status = $2 WHERE id = $1`, id, answered)
				return err
			}); err != nil {
				t.Fatalf("recording the outcome: %v", err)
			}

			if err := calendarSink(e).CancelMeeting(calendarOwnerCtx(e, e.AdminUser), meetingKey, meetingStart); err != nil {
				t.Fatalf("cancelling an answered meeting: %v — a sync must survive one", err)
			}
			if status, _ := readMeetingStatus(t, e, id); status != answered {
				t.Errorf("meeting_status = %q, want the recorded %q kept — a sync must not "+
					"overwrite what somebody reported about a meeting that happened", status, answered)
			}
		})
	}
}

// A Sink composed WITHOUT the seam captures meetings and cancels none — what
// every fixture is, and what a deployment composing capture without the timeline
// gets. It must not fail either.
func TestASinkWithoutTheSeamDoesNotFail(t *testing.T) {
	e := integration.Setup(t)
	id := captureMeeting(t, e, e.AdminUser)

	bare := capture.NewSink(e.DB())
	if err := bare.CancelMeeting(calendarOwnerCtx(e, e.AdminUser), meetingKey, meetingStart); err != nil {
		t.Fatalf("cancelling through a Sink with no canceller: %v", err)
	}
	if status, set := readMeetingStatus(t, e, id); set {
		t.Errorf("meeting_status = %q, want a Sink without the seam to write nothing", status)
	}
}

// A connector may cancel only its OWN provider's meetings.
//
// The natural key is what finds the row, and it is an argument rather than
// something the acting principal implies — so without this refusal any connector
// principal could close another's meetings by naming its source system. It is
// the same rule admitRecord states for a write ("a connector cannot claim to be
// another one"), and it binds here for the same reason.
func TestAConnectorCannotCancelAnotherConnectorsMeeting(t *testing.T) {
	e := integration.Setup(t)
	id := captureMeeting(t, e, e.AdminUser)

	// A Telegram sync, naming the calendar's key. Everything about this
	// principal is legitimate except the provider it is speaking for.
	impostor := principal.WithCorrelationID(
		principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7())
	impostor = principal.WithActor(impostor, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:telegram",
		UserID: e.AdminUser, OnBehalfOf: e.AdminUser,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"activity": {Create: true, Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})

	err := calendarSink(e).CancelMeeting(impostor, meetingKey, meetingStart)
	if err == nil {
		t.Fatal("a telegram connector cancelled a gcal meeting; a connector must act for its own provider only")
	}
	if status, set := readMeetingStatus(t, e, id); set {
		t.Errorf("meeting_status = %q after a refused cancellation, want the row untouched", status)
	}
}

// What the user actually sees: a cancelled meeting leaves Today's schedule.
//
// Every test above proves a WRITE. This one reads the lane that draws the rail,
// through the real seam over the real store, because the column and the screen
// are joined by a filter that could disagree with both — and a meeting the
// reader can still see is the whole complaint, however correct the row is.
func TestACancelledMeetingLeavesTodaysSchedule(t *testing.T) {
	e := integration.Setup(t)
	ctx := calendarOwnerCtx(e, e.AdminUser)
	captureMeeting(t, e, e.AdminUser)

	lane := attentionMeetings{store: activities.NewStore(e.DB())}
	from, until := meetingStart.Add(-time.Hour), meetingStart.Add(time.Hour)

	booked, err := lane.Today(ctx, from, until, 50, attention.TasksVisible, ids.UUID{})
	if err != nil {
		t.Fatalf("reading today's schedule: %v", err)
	}
	if len(booked) != 1 {
		t.Fatalf("the schedule carries %d meetings before the cancellation, want the booked one", len(booked))
	}

	if err := calendarSink(e).CancelMeeting(ctx, meetingKey, meetingStart); err != nil {
		t.Fatalf("cancelling the meeting: %v", err)
	}

	after, err := lane.Today(ctx, from, until, 50, attention.TasksVisible, ids.UUID{})
	if err != nil {
		t.Fatalf("re-reading today's schedule: %v", err)
	}
	if len(after) != 0 {
		t.Errorf("the schedule still carries %d meetings after the cancellation, want none — "+
			"this is the row a reader keeps seeing for a meeting that is not happening", len(after))
	}
}
