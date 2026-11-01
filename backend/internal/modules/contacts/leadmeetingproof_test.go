// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "testing"

// A captured meeting carries no status, and it proves a booking until someone
// records how it went. A lead who declined it proves nothing by it.
func TestMeetingProofReadsAnUnrecordedMeetingAsBooked(t *testing.T) {
	for status, want := range map[string]PromoteTrigger{
		"": TriggerMeetingBooked, "booked": TriggerMeetingBooked, "held": TriggerMeetingHeld, "canceled": "", "no_show": "",
	} {
		got, proves := meetingProof(status, false)
		if got != want || proves != (want != "") || meetingProves(status, false) != proves {
			t.Errorf("meeting status %q proves %q (%t), want %q", status, got, proves, want)
		}
		if got, proves := meetingProof(status, true); proves || got != "" {
			t.Errorf("a declined meeting with status %q proves %q", status, got)
		}
	}
}
