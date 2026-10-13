// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"log/slog"
	"testing"
)

// A lane that failed because the caller left is no fault an operator can act
// on. The same failure under a live request still is.
func TestALaneThatFailedBecauseTheCallerLeftIsNotLoggedAsAFault(t *testing.T) {
	gone, leave := context.WithCancel(context.Background())
	leave()

	if got := laneFailureLevel(gone); got >= slog.LevelWarn {
		t.Errorf("a caller who left was logged at %v, want below warn", got)
	}
	if got := laneFailureLevel(context.Background()); got != slog.LevelError {
		t.Errorf("a failure under a live request was logged at %v, want error", got)
	}
}
