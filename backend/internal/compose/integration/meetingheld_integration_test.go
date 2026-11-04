// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// meetingOutcomeRead is the part of a meeting its outcome is judged by.
type meetingOutcomeRead struct {
	MeetingStatus   *string `json:"meeting_status"`
	AwaitingOutcome bool    `json:"awaiting_outcome"`
}

// A meeting reported as held is held on the web form's door and on the
// agent's, and no longer asks for its outcome. One logged with no status asks
// on both doors: its past start alone is not attendance.
func TestAReportedMeetingIsHeldOverBothDoors(t *testing.T) {
	d := newTwoDoors(t, "meeting-held-doors", "read", "write")
	contact := anIDIn(t, d.restStatus(t, "POST", "/v1/contacts",
		AnyMap{"full_name": "Lunch Lior", "source": "manual"}, http.StatusCreated))
	yesterday := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)

	for _, report := range []struct {
		name     string
		status   any
		wantHeld bool
	}{
		{name: "held", status: "held", wantHeld: true},
		{name: "unreported", status: nil, wantHeld: false},
	} {
		for _, door := range []string{"http", "tool"} {
			t.Run(report.name+"/"+door, func(t *testing.T) {
				fields := map[string]any{
					"kind": "meeting", "subject": "Introductory lunch", "occurred_at": yesterday,
					"links": []map[string]any{{"entity_type": "contact", "entity_id": contact}},
				}
				if report.status != nil {
					fields["meeting_status"] = report.status
				}
				var made json.RawMessage
				if door == "http" {
					fields["source"] = "manual"
					made = d.restStatus(t, "POST", "/v1/activities", fields, http.StatusCreated)
				} else {
					made = d.tool(t, "log_activity", fields)
				}
				var read meetingOutcomeRead
				raw := d.restStatus(t, "GET", "/v1/activities/"+anIDIn(t, made), nil, http.StatusOK)
				if err := json.Unmarshal(raw, &read); err != nil {
					t.Fatal(err)
				}
				held := read.MeetingStatus != nil && *read.MeetingStatus == "held"
				if held != report.wantHeld || read.AwaitingOutcome == report.wantHeld {
					t.Fatalf("read back meeting_status %v, awaiting_outcome %v; want held=%v and awaiting_outcome=%v",
						read.MeetingStatus, read.AwaitingOutcome, report.wantHeld, !report.wantHeld)
				}
			})
		}
	}
}
