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

// deferringWorker is a worker whose model call the router answered "not now",
// returned the way the reading workers return it: through FaultContext.
type deferringWorker struct{ err error }

func (w deferringWorker) Work(ctx context.Context, _ *river.Job[CloseDateSweepArgs]) error {
	return jobs.FaultContext(ctx, w.err)
}

// Every registered worker passes through sessionBudgetWorker, so a deferral a
// worker hands back sleeps until the moment the router named instead of
// spending one of the job's tries.
func TestAJobToldNotNowSleepsUntilTheRouterSaysAndSpendsNoTry(t *testing.T) {
	now := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	cases := map[string]struct {
		err  error
		want time.Duration
	}{
		"budget used up": {
			err:  &ai.BudgetDeferralError{Task: ai.TaskStageEvidenceExtract, NextAttemptAt: now.Add(11 * 24 * time.Hour)},
			want: 11 * 24 * time.Hour,
		},
		"provider down": {
			err:  &ai.ProviderDownError{Provider: "p", Health: model.HealthDown, RetryAfter: now.Add(90 * time.Second)},
			want: 90 * time.Second,
		},
		"moment already past": {
			err:  &ai.BudgetDeferralError{Task: ai.TaskDocumentExtract, NextAttemptAt: now.Add(-time.Minute)},
			want: 0,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			worker := sessionBudgetWorker[CloseDateSweepArgs]{work: deferringWorker{err: tc.err}, now: func() time.Time { return now }}
			err := worker.Work(t.Context(), &river.Job[CloseDateSweepArgs]{})
			snooze, ok := errors.AsType[*river.JobSnoozeError](err)
			if !ok {
				t.Fatalf("err = %v, want a snooze: a failed try is retried at once and the job is dropped after its last", err)
			}
			if snooze.Duration != tc.want {
				t.Errorf("snoozed %s, want %s", snooze.Duration, tc.want)
			}
		})
	}
	other := errors.New("the database went away")
	worker := sessionBudgetWorker[CloseDateSweepArgs]{work: deferringWorker{err: other}, now: func() time.Time { return now }}
	if err := worker.Work(t.Context(), &river.Job[CloseDateSweepArgs]{}); !errors.Is(err, other) {
		t.Errorf("err = %v, want any other fault returned for River to retry", err)
	}
}
