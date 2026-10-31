// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "testing"

// A captured meeting carries no status, and it proves a booking until someone
// records how it went.
func TestMeetingProofReadsAnUnrecordedMeetingAsBooked(t *testing.T) {
	for status, want := range map[string]PromoteTrigger{
		"": TriggerMeetingBooked, "booked": TriggerMeetingBooked, "held": TriggerMeetingHeld, "canceled": "", "no_show": "",
	} {
		got, proves := meetingProof(status)
		if got != want || proves != (want != "") || meetingProves(status) != proves {
			t.Errorf("meeting status %q proves %q (%t), want %q", status, got, proves, want)
		}
	}
}
