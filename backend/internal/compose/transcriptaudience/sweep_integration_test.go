// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package transcriptaudience_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/transcriptaudience"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
)

// Guests added to many meetings reach every transcript in one sweep, even when
// more transcripts drifted than one transaction takes.
func TestOneSweepReachesEveryDriftedTranscript(t *testing.T) {
	e := integration.Setup(t)
	booked := string(crmcontracts.ActivityMeetingStatusBooked)
	sweep := transcriptaudience.Sweep{Pool: e.Pool, Batch: 3}
	transcripts := sweep.Batch + 1
	for i := range transcripts {
		meeting, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: "meeting", Source: "manual", MeetingStatus: &booked,
		})
		if err != nil {
			t.Fatalf("logging meeting %d: %v", i, err)
		}
		wire := fmt.Sprintf(`{"kind": "meeting", "source": "manual", "source_system": "transcript",
			"subject": "Review call", "body": "Buyer: we renew.", "records_meeting_id": %q}`, meeting.Id)
		var req crmcontracts.CreateActivityRequest
		if err := json.Unmarshal([]byte(wire), &req); err != nil {
			t.Fatalf("decoding the transcript: %v", err)
		}
		in, err := activities.LogActivityInputFrom(req)
		if err != nil {
			t.Fatalf("reading the transcript: %v", err)
		}
		if _, _, err := e.Activities.LogActivity(e.Admin(), in); err != nil {
			t.Fatalf("logging transcript %d: %v", i, err)
		}
	}
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, user_id, role)
		SELECT m.id, $1, 'attendee' FROM activity m WHERE EXISTS (SELECT 1 FROM activity t WHERE t.source_activity_id = m.id)`,
		e.Rep1)

	if err := sweep.Workspace(t.Context(), e.WS); err != nil {
		t.Fatalf("sweeping: %v", err)
	}

	reached := e.WsCount(t, `SELECT count(*) FROM activity t JOIN activity_participant p ON p.activity_id = t.id
		WHERE t.source_activity_id IS NOT NULL AND p.user_id = $1`, e.Rep1)
	if reached != transcripts {
		t.Fatalf("the guest reached %d of %d transcripts in one sweep", reached, transcripts)
	}
}
