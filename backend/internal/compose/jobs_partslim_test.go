// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture"
)

// scriptedSlim answers batches from a script and counts how many were asked
// for, advancing a fake clock by step on each one.
type scriptedSlim struct {
	script []capture.PartSlimResult
	err    error
	asked  int
	clock  time.Time
	step   time.Duration
}

func (s *scriptedSlim) batch(context.Context) (capture.PartSlimResult, error) {
	s.asked++
	s.clock = s.clock.Add(s.step)
	if s.asked > len(s.script) {
		return capture.PartSlimResult{}, s.err
	}
	return s.script[s.asked-1], nil
}

func (s *scriptedSlim) now() time.Time { return s.clock }

func full(slimmed int) capture.PartSlimResult {
	return capture.PartSlimResult{Considered: capture.PartSlimBatch, Slimmed: slimmed, Freed: int64(slimmed) * 1000}
}

func TestAPassKeepsTakingBatchesUntilOneComesBackShort(t *testing.T) {
	s := &scriptedSlim{script: []capture.PartSlimResult{full(50), full(40), {Considered: 7, Slimmed: 3, Freed: 3000}}, step: time.Second}
	got, batches, err := drainPartSlim(t.Context(), s.batch, time.Hour, s.now)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if batches != 3 || s.asked != 3 {
		t.Fatalf("batches = %d, asked = %d; want 3 and 3", batches, s.asked)
	}
	want := capture.PartSlimResult{Considered: 2*capture.PartSlimBatch + 7, Slimmed: 93, Freed: 93000}
	if got != want {
		t.Fatalf("total = %+v, want %+v", got, want)
	}
}

func TestAPassStopsAtItsBudgetWhileTheBacklogIsStillFull(t *testing.T) {
	script := make([]capture.PartSlimResult, 100)
	for i := range script {
		script[i] = full(50)
	}
	s := &scriptedSlim{script: script, step: time.Minute}
	_, batches, err := drainPartSlim(t.Context(), s.batch, capturePartSlimDrainBudget, s.now)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if batches != 4 {
		t.Fatalf("batches = %d, want 4: the budget is checked after each batch", batches)
	}
}

func TestAPassFallsBackToOneBatchWhenTheStoreCannotVouch(t *testing.T) {
	unproved := full(10)
	unproved.Unproved = 2
	s := &scriptedSlim{script: []capture.PartSlimResult{unproved, full(50)}, step: time.Second}
	got, batches, err := drainPartSlim(t.Context(), s.batch, time.Hour, s.now)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if batches != 1 || got.Unproved != 2 {
		t.Fatalf("batches = %d, unproved = %d; want 1 and 2", batches, got.Unproved)
	}
}

func TestAPassReturnsTheFaultWithWhatItDidBeforeIt(t *testing.T) {
	boom := errors.New("store went away")
	s := &scriptedSlim{script: []capture.PartSlimResult{full(50)}, err: boom, step: time.Second}
	got, batches, err := drainPartSlim(t.Context(), s.batch, time.Hour, s.now)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if batches != 2 || got.Slimmed != 50 {
		t.Fatalf("batches = %d, slimmed = %d; want 2 and 50", batches, got.Slimmed)
	}
}

func TestAPassStopsWhenItsContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	s := &scriptedSlim{script: []capture.PartSlimResult{full(50), full(50)}, step: time.Second}
	cancelling := func(ctx context.Context) (capture.PartSlimResult, error) {
		cancel()
		return s.batch(ctx)
	}
	_, batches, err := drainPartSlim(ctx, cancelling, time.Hour, s.now)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want %v", err, context.Canceled)
	}
	if batches != 1 {
		t.Fatalf("batches = %d, want 1", batches)
	}
}
