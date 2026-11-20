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

// Every worker is registered through sessionBudgetWorker. A deferral a worker
// returns as it came waits as a snooze within the reschedule bounds, and
// spends none of the job's tries.
func TestAJobToldNotNowSleepsAndSpendsNoTry(t *testing.T) {
	// Instants no skewed clock comes near, so the wait is capped or floored.
	far := time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	other := errors.New("the database went away")
	for name, tc := range map[string]struct {
		err  error
		want time.Duration
	}{
		"budget used up": {&ai.BudgetDeferralError{NextAttemptAt: far}, 15 * time.Minute},
		"provider down":  {&ai.ProviderDownError{Health: model.HealthDown, RetryAfter: far}, 15 * time.Minute},
		"moment past":    {&ai.BudgetDeferralError{NextAttemptAt: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)}, time.Second},
		"other fault":    {other, -1},
		"tripping call":  {&ai.ProviderDownError{Health: model.HealthDown, RetryAfter: far, Cause: other}, -1},
	} {
		work := river.WorkFunc(func(context.Context, *river.Job[CloseDateSweepArgs]) error { return tc.err })
		err := sessionBudgetWorker[CloseDateSweepArgs]{work: work}.Work(t.Context(), &river.Job[CloseDateSweepArgs]{})
		snooze, snoozed := errors.AsType[*river.JobSnoozeError](err)
		if tc.want < 0 && (snoozed || !errors.Is(err, tc.err)) || tc.want >= 0 && (!snoozed || snooze.Duration != tc.want) {
			t.Errorf("%s: err = %v, want a snooze of %s (negative: the fault itself, for River to retry)", name, err, tc.want)
		}
	}
}
