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

// A status sent as null removes the answer. A canceled meeting asks again,
// the history ends on booked, and one event reports the clear.
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
	id := bookMeeting(t, e, "Harbor review", clocktest.Now(t).Add(-48*time.Hour))
	cleared, err := meetingStore(e).UpdateActivity(meetingCtx(e), id, UpdateActivityInput{Clear: []string{auditFieldMeetingStatus}})
	if err != nil || cleared.MeetingStatus != nil {
		t.Fatalf("the first clear: status %v, err %v", cleared.MeetingStatus, err)
	}

	updates := func() (n int) {
		if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM audit_log
			WHERE entity_type = 'activity' AND entity_id = $1 AND action = 'update'`, id.UUID).Scan(&n); err != nil {
			t.Fatalf("counting audit rows: %v", err)
		}
		return n
	}
	before := updates()
	if _, err := meetingStore(e).UpdateActivity(meetingCtx(e), id, UpdateActivityInput{Clear: []string{auditFieldMeetingStatus}}); err != nil {
		t.Fatalf("clearing an unset status: %v", err)
	}
	if after := updates(); after != before {
		t.Errorf("a clear of nothing wrote %d audit rows, want none", after-before)
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
