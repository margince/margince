// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "testing"

// A mailed card's import touches no model and no network, so a failure is
// either a transient database hiccup or a deterministic refusal — a
// malformed card, a bug — that answers identically on every attempt. River's
// own default would retry the deterministic case 25 times across hours of
// exponential backoff for no different an answer than the first attempt gave.
func TestVCardIngestDeclaresABoundedRetryLadder(t *testing.T) {
	got := vcardIngestInsertOpts().MaxAttempts
	if got != vcardIngestMaxAttempts {
		t.Fatalf("enqueued MaxAttempts = %d, want the declared ladder %d", got, vcardIngestMaxAttempts)
	}
	if got <= 0 || got >= 25 {
		t.Fatalf("MaxAttempts = %d, want a positive bound well below River's own default of 25", got)
	}
}
