// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"slices"
	"testing"
)

// The verdicts that close a thread's opened messages, stated as a spec. A
// verdict dropped from closingVerdicts, or one added, fails here.
func TestOnlyHoldingVerdictsCloseOpenedMessages(t *testing.T) {
	t.Parallel()
	for status, closes := range map[string]bool{
		VerdictHeld: true, VerdictUnsure: true, VerdictHeldByOwner: true,
		VerdictCleared: false, VerdictSharedByOwner: false, VerdictPending: false,
	} {
		if got := ClosesOpenMessages(status); got != closes {
			t.Errorf("ClosesOpenMessages(%q) = %v, want %v", status, got, closes)
		}
		if got := holdingVerdict(status); got != closes {
			t.Errorf("holdingVerdict(%q) = %v, want %v: the birth hold and the sibling stamp read one list", status, got, closes)
		}
		taken := siblingStatusesTaken(status)
		for _, opened := range []string{VerdictCleared, VerdictSharedByOwner} {
			if slices.Contains(taken, opened) != closes {
				t.Errorf("a %s verdict takes a %s message = %v, want %v", status, opened, !closes, closes)
			}
		}
	}
}
