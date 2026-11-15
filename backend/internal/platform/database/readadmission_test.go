// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"golang.org/x/sync/semaphore"
)

func TestReadAdmissionWaitsWithoutBorrowingAndContinuesWhenReleased(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		admission := &readAdmission{slots: semaphore.NewWeighted(1)}
		if err := admission.acquire(t.Context()); err != nil {
			t.Fatal(err)
		}
		ctx, meter := WithStatementMeter(t.Context())
		finished := make(chan error, 1)
		go func() { finished <- admission.acquire(ctx) }()
		synctest.Wait()
		if admission.waiters.Load() != 1 || meter.Cost().Acquires != 0 {
			t.Fatal("queued read borrowed from the pool")
		}
		admission.slots.Release(1)
		if err := <-finished; err != nil {
			t.Fatal(err)
		}
		admission.slots.Release(1)
		if cost := meter.Cost(); cost.Admissions != 1 || cost.Acquires != 0 {
			t.Fatalf("cost=%+v", cost)
		}
	})
}

func TestReadAdmissionCancellationAndQueueDeadlineReleaseWaiters(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(map[bool]string{false: "deadline", true: "caller cancellation"}[canceled], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				admission := &readAdmission{slots: semaphore.NewWeighted(1)}
				if err := admission.acquire(t.Context()); err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				ctx, meter := WithStatementMeter(ctx)
				finished := make(chan error, 1)
				go func() { finished <- admission.acquire(ctx) }()
				synctest.Wait()
				if admission.waiters.Load() != 1 {
					t.Fatal("read did not queue")
				}
				want := context.DeadlineExceeded
				if canceled {
					want = context.Canceled
					cancel()
				}
				if err := <-finished; !errors.Is(err, want) {
					t.Fatalf("got %v, want %v", err, want)
				}
				if !canceled && meter.Cost().AdmissionWait != readAdmissionTimeout {
					t.Fatalf("queue wait=%s, want %s", meter.Cost().AdmissionWait, readAdmissionTimeout)
				}
				if admission.waiters.Load() != 0 {
					t.Fatal("failed read retained its queue position")
				}
				if cost := meter.Cost(); cost.Admissions != 1 || cost.Acquires != 0 {
					t.Fatalf("cost=%+v", cost)
				}
				admission.slots.Release(1)
				if !admission.slots.TryAcquire(1) {
					t.Fatal("canceled read leaked capacity")
				}
				admission.slots.Release(1)
			})
		})
	}
}
