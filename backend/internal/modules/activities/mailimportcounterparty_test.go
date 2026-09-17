// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import "testing"

// Who an imported message was with, derived the way capture derives it.
//
// The outbound cases are the ones that matter. A sender is routinely on their
// own To line — anyone who copies themselves, every list they are subscribed
// to — and taking the first recipient blindly records the workspace as its own
// counterparty, which reads as correspondence with nobody.
func TestTheCounterpartyIsTheOtherSide(t *testing.T) {
	ours := func(addr string) bool { return addr == "us@example.test" }
	never := func(string) bool { return false }

	for _, tc := range []struct {
		name      string
		direction string
		parts     mailParticipants
		owned     func(string) bool
		want      string
	}{
		{
			name:      "an inbound message is with its sender",
			direction: "inbound",
			parts:     mailParticipants{From: "them@example.test", To: []string{"us@example.test"}},
			owned:     ours,
			want:      "them@example.test",
		},
		{
			name:      "an outbound message is with its recipient",
			direction: "outbound",
			parts:     mailParticipants{From: "us@example.test", To: []string{"them@example.test"}},
			owned:     ours,
			want:      "them@example.test",
		},
		{
			name:      "a sender who copied themselves is skipped over",
			direction: "outbound",
			parts:     mailParticipants{From: "us@example.test", To: []string{"us@example.test", "them@example.test"}},
			owned:     ours,
			want:      "them@example.test",
		},
		{
			name:      "an outbound message to nobody but ourselves has no counterparty",
			direction: "outbound",
			parts:     mailParticipants{From: "us@example.test", To: []string{"us@example.test"}},
			owned:     ours,
			want:      "",
		},
		{
			name:      "an outbound message with no recipients has no counterparty",
			direction: "outbound",
			parts:     mailParticipants{From: "us@example.test"},
			owned:     ours,
			want:      "",
		},
		{
			// When nothing is known to be ours, every address is somebody
			// else's and the first recipient stands — the answer capture
			// reaches for a mailbox whose owner it has not learned yet.
			name:      "an unknown sending side takes the first recipient",
			direction: "outbound",
			parts:     mailParticipants{From: "us@example.test", To: []string{"them@example.test"}},
			owned:     never,
			want:      "them@example.test",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := importedCounterparty(tc.direction, tc.parts, tc.owned); got != tc.want {
				t.Fatalf("counterparty = %q, want %q", got, tc.want)
			}
		})
	}
}
