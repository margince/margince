// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/jobs"
)

func TestSessionBudgetSnoozesBeforeBorrowingAConnection(t *testing.T) {
	ctx := context.WithValue(context.Background(), sessionBudgetKey{}, make(chan struct{}))
	_, held, err := holdSessionLock(ctx, nil, "test:budget", slog.New(slog.NewTextHandler(io.Discard, nil)))
	var snooze *river.JobSnoozeError
	if held || !errors.As(jobs.FaultContext(ctx, err), &snooze) {
		t.Fatalf("held=%v error=%v want snooze without a pool", held, err)
	}
}

func TestSessionSlotsAreSharedAndReturned(t *testing.T) {
	ctx := context.WithValue(context.Background(), sessionBudgetKey{}, make(chan struct{}, 1))
	release, err := takeSessionSlot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := takeSessionSlot(ctx); err == nil {
		t.Fatal("two locks borrowed one slot")
	}
	release()
	again, err := takeSessionSlot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	again()
}
