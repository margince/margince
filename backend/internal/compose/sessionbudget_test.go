// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/ports/model"
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

// Every worker is registered through sessionBudgetWorker, so a deferral sleeps
// until the moment the router named instead of spending one of the job's tries.
func TestAJobToldNotNowSleepsUntilTheRouterSaysAndSpendsNoTry(t *testing.T) {
	now := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	other := errors.New("the database went away")
	for name, tc := range map[string]struct {
		err  error
		want time.Duration
	}{
		"budget used up": {&ai.BudgetDeferralError{NextAttemptAt: now.Add(264 * time.Hour)}, 264 * time.Hour},
		"provider down":  {&ai.ProviderDownError{Health: model.HealthDown, RetryAfter: now.Add(90 * time.Second)}, 90 * time.Second},
		"moment past":    {&ai.BudgetDeferralError{NextAttemptAt: now.Add(-time.Minute)}, 0},
		"other fault":    {other, -1},
	} {
		// The worker returns the router's "not now" as the reading workers do.
		work := river.WorkFunc(func(ctx context.Context, _ *river.Job[CloseDateSweepArgs]) error {
			return jobs.FaultContext(ctx, tc.err)
		})
		worker := sessionBudgetWorker[CloseDateSweepArgs]{work: work, now: func() time.Time { return now }}
		err := worker.Work(t.Context(), &river.Job[CloseDateSweepArgs]{})
		snooze, snoozed := errors.AsType[*river.JobSnoozeError](err)
		if tc.want < 0 && (snoozed || !errors.Is(err, other)) || tc.want >= 0 && (!snoozed || snooze.Duration != tc.want) {
			t.Errorf("%s: err = %v, want a snooze of %s (negative: the fault itself, for River to retry)", name, err, tc.want)
		}
	}
}
