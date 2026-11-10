// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

// What a buyer may do in each room state, and what the published list shows.
// Both are pure functions of the state as read, so the rules are held here
// rather than only through the HTTP scenario. Expiry is roomStateSQL's, and the
// expiry scenarios in the integration suite hold it.

import "testing"

func TestBuyerAccessFollowsTheRoomState(t *testing.T) {
	cases := []struct {
		name  string
		state string
		want  string
	}{
		{"draft before anything is published still counts as live access", stateDraft, accessLive},
		{"building", "building", accessLive},
		{"ready", "ready", accessLive},
		{"publishing", "publishing", accessLive},
		{"live", stateLive, accessLive},
		{"paused", statePaused, accessPaused},
		{"closed keeps reading", stateClosed, accessClosed},
		{"expired", stateExpired, accessExpired},
		{"archived", stateArchived, accessExpired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := (roomStanding{state: tc.state}).access(); got != tc.want {
				t.Fatalf("access(%s) = %s, want %s", tc.state, got, tc.want)
			}
		})
	}
	if servesContent(accessPaused) || servesContent(accessExpired) {
		t.Fatal("a paused or expired room must serve no content")
	}
	if !servesContent(accessLive) || !servesContent(accessClosed) {
		t.Fatal("a live or closed room serves its content")
	}
}
