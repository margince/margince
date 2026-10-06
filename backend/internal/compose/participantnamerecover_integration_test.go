// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The attendee name recovery re-reads a stored invitation for the names its
// attendee rows were written without, reading each original only when its
// meeting's turn comes.

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// seedNamelessMeeting lands a Google meeting whose invitation names its guest,
// then clears the name off the attendee rows the way a row written before the
// column existed carries none.
func seedNamelessMeeting(t *testing.T, e *integration.Env, eventID string) ids.UUID {
	t.Helper()
	starts := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	raw, err := json.Marshal(map[string]any{
		"id": eventID, "status": "confirmed", "summary": "Consulting Monthly",
		"start":     map[string]string{"dateTime": starts.Format(time.RFC3339)},
		"organizer": map[string]string{"email": "client@acme.test"},
		"attendees": []map[string]any{
			{"email": "sam@acme.test", "displayName": "Sam Client", "responseStatus": "accepted"},
			{"email": backfillOwner, "self": true, "responseStatus": "accepted"},
		},
	})
	if err != nil {
		t.Fatalf("marshalling the invitation: %v", err)
	}
	ref, err := capture.NewSink(e.DB()).Upsert(calendarOwnerCtx(e, e.AdminUser), connector.NormalizedRecord{
		EntityType: "activity",
		NaturalKey: connector.NaturalKey{SourceSystem: calendarSystem, SourceID: eventID},
		Fields: capture.ActivityFields{
			Kind: "meeting", Subject: "Consulting Monthly", OccurredAt: starts,
		},
		Source:     calendarSystem + ":" + eventID,
		CapturedBy: "connector:" + calendarSystem,
		Raw:        raw,
	})
	if err != nil {
		t.Fatalf("seeding the meeting %s: %v", eventID, err)
	}
	if _, err := integration.OwnerConn(t).Exec(context.Background(), `
		INSERT INTO activity_participant (activity_id, role, address)
		SELECT $1, 'attendee', 'sam@acme.test'
		 WHERE NOT EXISTS (SELECT 1 FROM activity_participant
		                    WHERE activity_id = $1 AND address = 'sam@acme.test')`, ref.ID); err != nil {
		t.Fatalf("ensuring the guest's attendee row: %v", err)
	}
	if _, err := integration.OwnerConn(t).Exec(context.Background(),
		`UPDATE activity_participant SET display_name = NULL WHERE activity_id = $1`, ref.ID); err != nil {
		t.Fatalf("clearing the attendee names: %v", err)
	}
	return ref.ID
}

// guestName reads the name the guest's attendee row carries, NULL as "".
func guestName(t *testing.T, activity ids.UUID) (string, bool) {
	t.Helper()
	var name *string
	if err := integration.OwnerConn(t).QueryRow(context.Background(), `
		SELECT display_name FROM activity_participant
		 WHERE activity_id = $1 AND address = 'sam@acme.test' LIMIT 1`, activity).Scan(&name); err != nil {
		t.Fatalf("reading the guest's attendee row: %v", err)
	}
	if name == nil {
		return "", false
	}
	return *name, true
}

// The recovery reads the invitation it names by id and fills the guest's name
// in, and a meeting it has settled is not offered again.
func TestNameRecoveryFillsTheNameFromTheStoredInvitation(t *testing.T) {
	e := integration.Setup(t)
	meeting := seedNamelessMeeting(t, e, "evt-names")

	settled, err := recoverAttendeeNamesBatch(e.Admin(), e.Pool, 10, slog.Default())
	if err != nil {
		t.Fatalf("recoverAttendeeNamesBatch: %v", err)
	}
	if settled != 1 {
		t.Errorf("settled = %d, want the one nameless meeting", settled)
	}
	if name, set := guestName(t, meeting); !set || name != "Sam Client" {
		t.Errorf("the guest's name = %q (set %v), want the invitation's %q", name, set, "Sam Client")
	}
	again, err := recoverAttendeeNamesBatch(e.Admin(), e.Pool, 10, slog.Default())
	if err != nil {
		t.Fatalf("the second pass: %v", err)
	}
	if again != 0 {
		t.Errorf("the second pass settled %d, want 0 — the rows are no longer nameless", again)
	}
}

// A meeting whose original has gone by its turn settles nothing: its rows keep
// their NULL, and it is not counted as progress.
func TestNameRecoveryPassesOverAnOriginalThatIsGone(t *testing.T) {
	e := integration.Setup(t)
	meeting := seedNamelessMeeting(t, e, "evt-gone")

	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		done, err := recoverOneMeetingsNames(e.Admin(), tx, nameRecoveryCandidate{
			activityID: ids.From[ids.ActivityKind](meeting), source: calendarSystem, rawCaptureID: ids.NewV7(),
		})
		if err != nil {
			return err
		}
		if done {
			t.Error("a meeting with no original on file was reported as settled")
		}
		return nil
	}); err != nil {
		t.Fatalf("settling a meeting with no original: %v", err)
	}
	if name, set := guestName(t, meeting); set {
		t.Errorf("the guest's row was named %q from an original nobody read", name)
	}
}

// An original the parser cannot read still settles the meeting, with nobody
// named, so the rows are not re-offered on every tick.
func TestNamedPartiesOfAnswersNobodyForAnUnreadableOriginal(t *testing.T) {
	for name, tc := range map[string]struct {
		source  string
		payload []byte
	}{
		"not json":         {source: calendarSystem, payload: []byte("{")},
		"unknown format":   {source: "imap", payload: []byte(`"x"`)},
		"not an event":     {source: calendarSystem, payload: []byte(`"not an event"`)},
		"graph not events": {source: sourceGraphCal, payload: []byte(`"not an event"`)},
	} {
		if got := namedPartiesOf(tc.source, tc.payload); len(got) != 0 {
			t.Errorf("%s: named %v, want nobody", name, got)
		}
	}
}
