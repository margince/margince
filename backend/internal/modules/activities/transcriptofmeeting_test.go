// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"encoding/json"
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestOnlyAMeetingTranscriptMayNameTheMeetingItRecords(t *testing.T) {
	const meeting = `"records_meeting_id": "0190a1b2-0000-7000-8000-000000000001"`
	for name, wire := range map[string]string{
		"typed meeting notes": `{"kind": "meeting", "source": "manual", "body": "Notes", ` + meeting + `}`,
		"a call transcript":   `{"kind": "call", "source": "manual", "source_system": "transcript", "body": "A: hi", ` + meeting + `}`,
		"a transcript with a task source": `{"kind": "meeting", "source": "manual", "source_system": "transcript", "body": "A: hi",
			"source_activity_id": "0190a1b2-0000-7000-8000-000000000002", ` + meeting + `}`,
	} {
		var req crmcontracts.CreateActivityRequest
		if err := json.Unmarshal([]byte(wire), &req); err != nil {
			t.Fatalf("%s: decoding: %v", name, err)
		}
		_, err := LogActivityInputFrom(req)
		var refused *followUpFieldError
		if !errors.As(err, &refused) || refused.Field != fieldRecordsMeetingID {
			t.Errorf("%s answered %v, want a refusal on %s", name, err, fieldRecordsMeetingID)
		}
	}
}

func TestAMeetingTranscriptStoresTheMeetingItRecordsAsItsSource(t *testing.T) {
	var req crmcontracts.CreateActivityRequest
	wire := `{"kind": "meeting", "source": "manual", "source_system": "transcript", "body": "A: hi",
		"records_meeting_id": "0190a1b2-0000-7000-8000-000000000001"}`
	if err := json.Unmarshal([]byte(wire), &req); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	in, err := LogActivityInputFrom(req)
	if err != nil {
		t.Fatalf("mapping: %v", err)
	}
	if source := in.sourceActivity(); source == nil || source.String() != "0190a1b2-0000-7000-8000-000000000001" {
		t.Errorf("the stored source is %v, want the recorded meeting", source)
	}
}

// Without an outcome, a meeting transcript naming a task would read as the
// record of another meeting (meetingoutcome.RecordsMeetingSQL).
func TestAMeetingTranscriptAnsweringATaskCarriesItsOutcome(t *testing.T) {
	const answers = `{"kind": "meeting", "source": "manual", "source_system": "transcript", "body": "A: hi",
		"source_activity_id": "0190a1b2-0000-7000-8000-000000000002"`
	for wire, refused := range map[string]bool{
		answers + `}`:                           true,
		answers + `, "meeting_status": "held"}`: false,
	} {
		var req crmcontracts.CreateActivityRequest
		if err := json.Unmarshal([]byte(wire), &req); err != nil {
			t.Fatalf("decoding: %v", err)
		}
		_, err := LogActivityInputFrom(req)
		var fault *followUpFieldError
		if got := errors.As(err, &fault) && fault.Field == "meeting_status"; got != refused {
			t.Errorf("%s answered %v, want refused=%t", wire, err, refused)
		}
	}
}
