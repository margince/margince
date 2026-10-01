// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package backoff_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/backoff"
)

func TestASleepEndsWithTheCallerInsteadOfTheTimer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := backoff.Sleep(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("Sleep on a cancelled caller = %v, want context.Canceled", err)
	}
}

func TestASleepThatRunsOutReportsNoError(t *testing.T) {
	if err := backoff.Sleep(context.Background(), 0); err != nil {
		t.Fatalf("Sleep(0) = %v, want nil", err)
	}
}
