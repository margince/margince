// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package orderedfetch

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/sync/semaphore"
)

func TestACancelledReservationTakesNoCapacity(t *testing.T) {
	budget := semaphore.NewWeighted(100)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Reserve(ctx, budget, 100); !errors.Is(err, context.Canceled) {
		t.Fatalf("reservation = %v, want cancellation", err)
	}
	if !budget.TryAcquire(100) {
		t.Fatal("a cancelled reservation kept budget")
	}
}

func TestEveryResultIsConsumedInIndexOrderWithinTheBudget(t *testing.T) {
	budget := semaphore.NewWeighted(30)
	walk := Walk[int]{Workers: 3, Lookahead: 3, Budget: budget, Reserve: 10, Retained: func(int) int64 { return 1 }}
	var got []int
	err := walk.InOrder(t.Context(), 20,
		func(_ context.Context, i int) (int, error) { return i * i, nil },
		func(i, v int, err error) error {
			if err != nil || v != i*i {
				t.Errorf("item %d = %d, %v", i, v, err)
			}
			got = append(got, i)
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	for i, index := range got {
		if index != i {
			t.Fatalf("consumed %v, want 0..19 in order", got)
		}
	}
	if len(got) != 20 || !budget.TryAcquire(30) {
		t.Fatalf("consumed %d items; budget returned: want all 20 and the whole budget back", len(got))
	}
}

func TestAConsumerErrorStopsTheWalkAndReturnsTheBudget(t *testing.T) {
	budget := semaphore.NewWeighted(20)
	stop := errors.New("database down")
	walk := Walk[int]{Workers: 2, Lookahead: 2, Budget: budget, Reserve: 10}
	consumed := 0
	err := walk.InOrder(t.Context(), 50,
		func(_ context.Context, i int) (int, error) { return i, nil },
		func(i, _ int, _ error) error {
			consumed++
			if i == 3 {
				return stop
			}
			return nil
		})
	if !errors.Is(err, stop) || consumed != 4 {
		t.Fatalf("err = %v after %d consumed, want the consumer's error after 4", err, consumed)
	}
	if !budget.TryAcquire(20) {
		t.Fatal("a stopped walk kept budget")
	}
}

func TestAHaltingFetchStopsTheFetchesBehindIt(t *testing.T) {
	limited := errors.New("rate limited")
	walk := Walk[int]{Workers: 1, Lookahead: 1, Halts: func(err error) bool { return errors.Is(err, limited) }}
	fetched := 0
	err := walk.InOrder(t.Context(), 20,
		func(_ context.Context, i int) (int, error) {
			fetched++
			if i == 2 {
				return 0, limited
			}
			return i, nil
		},
		// A consumer that lets the error pass still cannot walk past it.
		func(int, int, error) error { return nil })
	if !errors.Is(err, limited) {
		t.Fatalf("err = %v, want the halting fetch's error", err)
	}
	if fetched > 4 {
		t.Fatalf("%d fetches, want the walk to stop right after the halting one", fetched)
	}
}
