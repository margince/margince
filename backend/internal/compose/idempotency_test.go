// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a refusal means for the caller's key, which is not one answer.

import "testing"

// A refusal gives the key back so the caller can fix the call and retry. A
// success, or a refusal after a committed write, is never released.
//
// Asserted on the DECISION rather than on the database, because the database
// half is recordClaimOutcome and releaseClaim, each already held.
func TestARefusalReleasesTheKeyAndASuccessDoesNot(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		committed bool
		release   bool
	}{
		{"a refusal", 422, false, true},
		{"a server error", 500, false, true},
		{"a success", 200, false, false},
		{"a timeout after a commit", 503, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := claimIsReleased(tc.status, tc.committed); got != tc.release {
				t.Errorf("released = %v, want %v", got, tc.release)
			}
		})
	}
}
