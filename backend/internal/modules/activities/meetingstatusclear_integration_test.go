// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A status sent as null removes the answer: a canceled meeting asks again,
// the history ends on booked, and one event says the status was cleared.
func TestAMeetingStatusClearedWithNullAsksForTheOutcomeAgain(t *testing.T) {
	e := setupSend(t)
	id := bookMeeting(t, e, "Harbor review", clocktest.Now(t).Add(-48*time.Hour))
	moveTo(t, e, id, crmcontracts.ActivityMeetingStatusCanceled)

	cleared, err := meetingStore(e).UpdateActivity(meetingCtx(e), id, UpdateActivityInput{Clear: []string{auditFieldMeetingStatus}})
	if err != nil {
		t.Fatalf("clearing the status: %v", err)
	}
	if cleared.MeetingStatus != nil {
		t.Errorf("status after the clear = %q, want none", *cleared.MeetingStatus)
	}
	history := historyOf(t, e, id)
	if last := history[len(history)-1].Status; last != "booked" {
		t.Errorf("newest transition = %q, want booked", last)
	}
	var events int
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM event_outbox
		WHERE envelope->'entity'->>'id' = $1::text AND envelope->'payload'->'changed_fields'->>'meeting_status_cleared' = 'true'`,
		id.String()).Scan(&events); err != nil {
		t.Fatalf("reading the outbox: %v", err)
	}
	if events != 1 {
		t.Errorf("the clear raised %d meeting_status_cleared events, want 1", events)
	}
}

// Clearing a status that is already none writes nothing, so the version holds.
func TestClearingAnUnsetMeetingStatusChangesNothing(t *testing.T) {
	e := setupSend(t)
	subject := "Harbor review"
	start := clocktest.Now(t).Add(-48 * time.Hour)
	out, _, err := meetingStore(e).LogActivity(meetingCtx(e), LogActivityInput{
		Kind: string(crmcontracts.ActivityKindMeeting), Subject: &subject, OccurredAt: &start, Source: "manual",
	})
	if err != nil {
		t.Fatalf("logging the meeting: %v", err)
	}
	id := ids.From[ids.ActivityKind](ids.UUID(out.Id))

	again, err := meetingStore(e).UpdateActivity(meetingCtx(e), id, UpdateActivityInput{Clear: []string{auditFieldMeetingStatus}})
	if err != nil {
		t.Fatalf("clearing an unset status: %v", err)
	}
	if again.Version != out.Version {
		t.Errorf("version moved from %d to %d on a clear of nothing", out.Version, again.Version)
	}
}

// Only a meeting has a status, so a null on a note is the same refusal a value is.
func TestClearingTheMeetingStatusOfANoteIsRefused(t *testing.T) {
	e := setupSend(t)
	note, _, err := meetingStore(e).LogActivity(meetingCtx(e), LogActivityInput{
		Kind: string(crmcontracts.ActivityKindNote), Body: new("Called back"), Source: "manual",
	})
	if err != nil {
		t.Fatalf("logging the note: %v", err)
	}

	_, err = meetingStore(e).UpdateActivity(meetingCtx(e), ids.From[ids.ActivityKind](ids.UUID(note.Id)), UpdateActivityInput{Clear: []string{auditFieldMeetingStatus}})
	if _, ok := errors.AsType[*MeetingStatusKindError](err); !ok {
		t.Errorf("clearing the status of a note answered %v, want MeetingStatusKindError", err)
	}
}
