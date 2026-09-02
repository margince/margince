// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/riverqueue/river"
)

// The insert carries the declared constant, and that constant is itself a
// sane bound — an unset MaxAttempts is silently River's own default with no
// other symptom, which is what the second check would catch even if the
// first held by coincidence.
func TestVCardIngestDeclaresABoundedRetryLadder(t *testing.T) {
	got := vcardIngestInsertOpts().MaxAttempts
	if got != vcardIngestMaxAttempts {
		t.Fatalf("enqueued MaxAttempts = %d, want the declared ladder %d", got, vcardIngestMaxAttempts)
	}
	if got <= 0 || got >= river.MaxAttemptsDefault {
		t.Fatalf("MaxAttempts = %d, want a positive bound well below River's own default of %d", got, river.MaxAttemptsDefault)
	}
}
