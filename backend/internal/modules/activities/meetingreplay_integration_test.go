// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// A calendar redelivering the same meeting is the case where "a replay writes
// nothing" stops being idempotence and starts being a lie.
//
// The defect these exist against: logActivityInTx's (source_system, source_id)
// replay returned the existing row untouched, so a provider that first
// delivered a meeting as booked and later redelivered the same natural key as
// canceled got a 200 while the row kept rendering an upcoming meeting — and no
// activity.updated fired, so nothing downstream re-read it.

import (
	"context"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// deliverMeeting logs one meeting under a natural key, the way a calendar
// connector does. Returns the activity the call answered with.
func deliverMeeting(
	t *testing.T, e *sendEnv, key, subject string, start time.Time,
	status crmcontracts.ActivityMeetingStatus,
) crmcontracts.Activity {
	t.Helper()
	system, id, s := "calendar-test", key, string(status)
	out, _, err := meetingStore(e).LogActivity(meetingCtx(e), LogActivityInput{
		Kind:          string(crmcontracts.ActivityKindMeeting),
		Subject:       &subject,
		OccurredAt:    &start,
		MeetingStatus: &s,
		SourceSystem:  &system,
		SourceID:      &id,
		Source:        "sync",
	})
	if err != nil {
		t.Fatalf("delivering %s as %s: %v", key, status, err)
	}
	return out
}

// eventKinds reads the outbox verbs emitted for one activity, oldest first.
func eventKinds(t *testing.T, e *sendEnv, activityID ids.ActivityID) []string {
	t.Helper()
	rows, err := e.owner.Query(context.Background(),
		`SELECT envelope->>'type' FROM event_outbox
		  WHERE envelope->'entity'->>'id' = $1::text
		  ORDER BY id`, activityID.String())
	if err != nil {
		t.Fatalf("reading the outbox: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var kind string
		if err := rows.Scan(&kind); err != nil {
			t.Fatalf("scanning an event: %v", err)
		}
		out = append(out, kind)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the outbox: %v", err)
	}
	return out
}

// A meeting cancelled at the provider is cancelled here, on the redelivery of
// the natural key that carried it.
//
// The status is the ONE field a replay may move: booked → held / no_show /
// canceled is a vocabulary that exists to change over time, and nothing on this
// side of the connector can know it did.
func TestARedeliveredMeetingCarriesItsNewStatus(t *testing.T) {
	e := setupSend(t)
	start := time.Now().Add(48 * time.Hour)
	first := deliverMeeting(t, e, "evt-cancel-1", "Northgate review", start,
		crmcontracts.ActivityMeetingStatusBooked)
	id := ids.From[ids.ActivityKind](ids.UUID(first.Id))

	second := deliverMeeting(t, e, "evt-cancel-1", "Northgate review", start,
		crmcontracts.ActivityMeetingStatusCanceled)

	if ids.UUID(second.Id) != ids.UUID(first.Id) {
		t.Fatalf("the redelivery minted a second activity (%v, was %v) — the natural key "+
			"stopped being one", second.Id, first.Id)
	}
	if got := meetingStatusString(second.MeetingStatus); got != "canceled" {
		t.Errorf("the call answered %q, want canceled", got)
	}
	// The ROW, not merely what the call said: a caller reading the meeting
	// afterwards is what renders "upcoming" or does not.
	var stored string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT meeting_status FROM activity WHERE id = $1`, id.UUID).Scan(&stored); err != nil {
		t.Fatalf("reading the meeting back: %v", err)
	}
	if stored != "canceled" {
		t.Errorf("the stored meeting_status is %q, want canceled — the meeting goes on "+
			"rendering as upcoming and nothing says otherwise", stored)
	}

	// Both transitions, because the cancellation is a second event and not an
	// overwrite of the first — the same rule a human's PATCH follows.
	history := historyOf(t, e, id)
	if len(history) != 2 {
		t.Fatalf("recorded %d transitions, want 2 (booked, then canceled): %+v", len(history), history)
	}
	if history[1].Status != "canceled" {
		t.Errorf("second transition = %q, want canceled", history[1].Status)
	}

	// And it is announced. Without this the lead ladder and every other
	// consumer keep the meeting they last read, which is the booked one.
	kinds := eventKinds(t, e, id)
	activityUpdatedEvent := crmcontracts.PublicEventActivityUpdated{}.EventType()
	if len(kinds) != 2 || kinds[1] != activityUpdatedEvent {
		t.Errorf("events = %v, want the capture then an activity.updated — a status that "+
			"moved without an event is one nothing downstream re-reads", kinds)
	}
}

// A redelivery carrying the SAME status is the ordinary case, and it writes
// nothing.
//
// This is what keeps capture idempotent: an at-least-once connector redelivers
// constantly, and a transition recorded per delivery would make "booked" a
// countable event that happened four times.
func TestARedeliveredMeetingThatDidNotMoveWritesNothing(t *testing.T) {
	e := setupSend(t)
	start := time.Now().Add(48 * time.Hour)
	first := deliverMeeting(t, e, "evt-same-1", "Kestrel sync", start,
		crmcontracts.ActivityMeetingStatusBooked)
	id := ids.From[ids.ActivityKind](ids.UUID(first.Id))

	deliverMeeting(t, e, "evt-same-1", "Kestrel sync", start,
		crmcontracts.ActivityMeetingStatusBooked)

	if history := historyOf(t, e, id); len(history) != 1 {
		t.Errorf("recorded %d transitions, want 1 — a redelivery of the same status is the "+
			"same fact arriving twice, and counting it makes booked happen twice: %+v",
			len(history), history)
	}
	if kinds := eventKinds(t, e, id); len(kinds) != 1 {
		t.Errorf("events = %v, want only the capture — an update event for a change that "+
			"did not happen wakes every consumer for nothing", kinds)
	}
}

// A replay does not overwrite what a person wrote.
//
// The provider is authoritative for the meeting's lifecycle and for nothing
// else. A rep who corrected the subject keeps their correction when the
// calendar sends its own version again — which is why this moves the status
// alone rather than patching every mutable field.
func TestARedeliveryLeavesTheSubjectAPersonCorrected(t *testing.T) {
	e := setupSend(t)
	start := time.Now().Add(48 * time.Hour)
	first := deliverMeeting(t, e, "evt-subject-1", "mtg", start,
		crmcontracts.ActivityMeetingStatusBooked)
	id := ids.From[ids.ActivityKind](ids.UUID(first.Id))
	corrected := "Northgate — pricing walkthrough"
	if _, err := meetingStore(e).UpdateActivity(meetingCtx(e), id,
		UpdateActivityInput{Subject: &corrected}); err != nil {
		t.Fatalf("correcting the subject: %v", err)
	}

	deliverMeeting(t, e, "evt-subject-1", "mtg", start,
		crmcontracts.ActivityMeetingStatusHeld)

	var subject, status string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT subject, meeting_status FROM activity WHERE id = $1`, id.UUID).
		Scan(&subject, &status); err != nil {
		t.Fatalf("reading the meeting back: %v", err)
	}
	if subject != corrected {
		t.Errorf("subject = %q, want the rep's correction %q — the provider's own version "+
			"came back and overwrote a person's work", subject, corrected)
	}
	if status != "held" {
		t.Errorf("meeting_status = %q, want held", status)
	}
}
