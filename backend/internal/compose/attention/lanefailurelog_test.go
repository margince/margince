// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

// A lane that failed because the caller left is not a fault an operator can
// act on, while the same failure under a live request still is.
func TestALaneThatFailedBecauseTheCallerLeftIsNotLoggedAsAFault(t *testing.T) {
	var logged bytes.Buffer
	restore := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(restore) })

	gone, leave := context.WithCancel(context.Background())
	leave()
	logLaneFailure(gone, "a follow-up read failed", "error", errors.New("conn closed"))
	if logged.Len() != 0 {
		t.Fatalf("a caller who left was logged at warn or above: %s", logged.String())
	}

	logLaneFailure(context.Background(), "a follow-up read failed", "error", errors.New("boom"))
	if !strings.Contains(logged.String(), "level=ERROR") {
		t.Errorf("a failure under a live request was not logged as an error: %q", logged.String())
	}
}
