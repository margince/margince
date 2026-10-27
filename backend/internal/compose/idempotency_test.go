// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a refusal means for the caller's key, which is not one answer.

import "testing"

// A refusal gives the key back so the caller can fix the call and retry. A
// success is recorded for replay and never released.
//
// Asserted on the DECISION rather than on the database, because the database
// half is recordClaimOutcome and releaseClaim, each already held.
func TestARefusalReleasesTheKeyAndASuccessDoesNot(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		release bool
	}{
		{"a refusal", 422, true},
		{"a server error", 500, true},
		{"a success", 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := claimIsReleased(tc.status); got != tc.release {
				t.Errorf("released = %v, want %v", got, tc.release)
			}
		})
	}
}
