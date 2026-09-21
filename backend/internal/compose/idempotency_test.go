// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a refusal means for the caller's key, which is not one answer.

import "testing"

// A refusal that changed nothing gives the key back; one that changed something
// does not.
//
// The distinction is the whole of #1073. Every non-2xx released the claim, so
// the per-field split — which applies the agent-owned fields and then stages the
// human-owned residue — handed the key back after a staging failure that
// followed a committed write. The retry the caller is entitled to make then ran
// the applied half a second time under the same key, which is precisely what
// the key exists to prevent.
//
// Asserted on the DECISION rather than on the database, because the database
// half is recordClaimOutcome and releaseClaim, each already held: what was
// wrong was which of the two a refusal reached.
func TestARefusalReleasesTheKeyOnlyIfItChangedNothing(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		wrote   bool
		release bool
	}{
		{"a refusal that wrote nothing", 422, false, true},
		{"a refusal that had already written", 500, true, false},
		{"a success", 200, false, false},
		// A success is recorded whatever it reports, so the marker cannot make
		// a completed call retryable by accident.
		{"a success that wrote", 200, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := claimIsReleased(tc.status, tc.wrote); got != tc.release {
				t.Errorf("released = %v, want %v", got, tc.release)
			}
		})
	}
}
