// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A body sent as null empties the notes: the row reads back with no body at all.
func TestABodyClearedWithNullEmptiesTheNotes(t *testing.T) {
	e := setupSend(t)
	id := bookMeeting(t, e, "Harbor review", clocktest.Now(t).Add(-48*time.Hour))
	moveTo(t, e, id, crmcontracts.ActivityMeetingStatusHeld)
	if _, err := meetingStore(e).UpdateActivity(meetingCtx(e), id, UpdateActivityInput{Body: new("Budget approved")}); err != nil {
		t.Fatalf("writing the notes: %v", err)
	}

	cleared, err := meetingStore(e).UpdateActivity(meetingCtx(e), id, UpdateActivityInput{Clear: []string{fieldBody}})
	if err != nil {
		t.Fatalf("clearing the notes: %v", err)
	}
	if cleared.Body != nil {
		t.Errorf("body after the clear = %q, want none", *cleared.Body)
	}
}

// A note says something or it is not stored. Clearing the body of a note with
// no subject would leave an empty entry, so the store refuses it.
func TestClearingTheOnlyTextOfANoteIsRefused(t *testing.T) {
	e := setupSend(t)
	note, _, err := meetingStore(e).LogActivity(meetingCtx(e), LogActivityInput{
		Kind:   string(crmcontracts.ActivityKindNote),
		Body:   new("Called back"),
		Source: "manual",
	})
	if err != nil {
		t.Fatalf("logging the note: %v", err)
	}

	_, err = meetingStore(e).UpdateActivity(meetingCtx(e), ids.From[ids.ActivityKind](ids.UUID(note.Id)), UpdateActivityInput{Clear: []string{fieldBody}})
	var empty *EmptyEntryError
	if !errors.As(err, &empty) {
		t.Errorf("clearing the only text of a note answered %v, want EmptyEntryError", err)
	}
}
