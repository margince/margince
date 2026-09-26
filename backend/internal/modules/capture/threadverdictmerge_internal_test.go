// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import "testing"

// Merging two halves of a conversation never leaves a seat's verdict more open
// than either half: a hold wins from either side, pending wins over an opening
// answer, and two opening answers go back to the classifier.
func TestAMergedVerdictIsNeverMoreOpenThanEitherHalf(t *testing.T) {
	for _, c := range []struct {
		from, to         string
		keepFrom, reopen bool
	}{
		{VerdictHeld, VerdictCleared, true, false},
		{VerdictCleared, VerdictHeld, false, false},
		{VerdictHeldByOwner, VerdictHeld, true, false},
		{VerdictUnsure, VerdictSharedByOwner, true, false},
		{VerdictPending, VerdictCleared, true, false},
		{VerdictCleared, VerdictPending, false, false},
		{VerdictCleared, VerdictSharedByOwner, false, true},
		{VerdictSharedByOwner, VerdictSharedByOwner, false, true},
		{VerdictHeld, VerdictHeld, false, false},
	} {
		keepFrom, reopen := mergedVerdict(c.from, c.to)
		if keepFrom != c.keepFrom || reopen != c.reopen {
			t.Errorf("mergedVerdict(%s, %s) = keepFrom %v reopen %v, want %v %v",
				c.from, c.to, keepFrom, reopen, c.keepFrom, c.reopen)
		}
	}
}
