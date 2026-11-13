// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"testing"

	"github.com/jackc/pgx/v5"
)

// A transcript filed on the meeting it records is one engagement in the lead
// score, not a held meeting plus a booking. The pair is seeded in the shape the
// activities writer stores it (records_meeting_id).
func TestAMeetingAndItsTranscriptScoreTheLeadOnce(t *testing.T) {
	e := setupPromoteConsent(t)
	lead := e.seedLead(t, "pair@evidence.test")
	meeting := e.seedEvidence(t, &lead, heldMeeting)
	transcript := e.seedEvidence(t, &lead, evidenceSeed{kind: "meeting"})
	e.mustExec(t, `UPDATE activity SET source_system = 'transcript', source_activity_id = $2 WHERE id = $1`,
		transcript, meeting)

	var signals []BehavioralSignal
	if err := e.store.tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		signals, err = leadBehavioralSignals(e.ctx, tx, lead)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(signals) != 1 || signals[0].Kind != string(TriggerMeetingHeld) || signals[0].ActivityID != meeting {
		t.Errorf("the lead scores %+v, want the one held meeting", signals)
	}
}
