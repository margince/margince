// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// Scoring one round's runs on several workers at once. A run is two slow model
// calls and shares nothing. So a round takes as long as its slowest worker, not
// the sum of its runs.

import (
	"context"
	"fmt"
	"sync"
)

// defaultParallel is how many runs of one task are in flight when
// MARGINCE_AICERT_PARALLEL is unset.
const defaultParallel = 4

// parallelOrDefault applies RunnerConfig.Parallel's default and refuses a
// negative count, which is an operator typo rather than a request for none.
func parallelOrDefault(n int) (int, error) {
	if n == 0 {
		return defaultParallel, nil
	}
	if n < 1 {
		return 0, fmt.Errorf("aicert: runner: MARGINCE_AICERT_PARALLEL must be positive, got %d", n)
	}
	return n, nil
}

// scoreConcurrently measures jobs on up to workers goroutines and folds each
// result in job order, never completion order. The fold is where a run joins
// the record and the journal. A record built in completion order would differ
// between two runs of the same corpus.
//
// measure receives the worker's index, so each worker can own state that is not
// safe to share. A fold happens as soon as every job before it has folded, so a
// slow early run delays the journal but never reorders it.
//
// The first error cancels the jobs still running and is the one returned.
func scoreConcurrently[R any](ctx context.Context, jobs []runJob, workers int,
	measure func(ctx context.Context, worker int, job runJob) (R, error),
	fold func(job runJob, measured R) (RunResult, error),
) ([]RunResult, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type finished struct {
		at       int
		measured R
		err      error
	}
	next := make(chan int)
	done := make(chan finished)
	var wg sync.WaitGroup
	for worker := range min(workers, len(jobs)) {
		wg.Go(func() {
			for at := range next {
				measured, err := measure(ctx, worker, jobs[at])
				done <- finished{at: at, measured: measured, err: err}
			}
		})
	}
	go func() {
		defer close(next)
		for at := range jobs {
			select {
			case next <- at:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(done)
	}()

	results := make([]RunResult, len(jobs))
	waiting := make(map[int]R, workers)
	folded := 0
	var firstErr error
	for f := range done {
		if firstErr != nil {
			continue // draining, so no worker blocks on a send nobody reads
		}
		if f.err != nil {
			firstErr = f.err
			cancel()
			continue
		}
		waiting[f.at] = f.measured
		for firstErr == nil {
			measured, ready := waiting[folded]
			if !ready {
				break
			}
			delete(waiting, folded)
			result, err := fold(jobs[folded], measured)
			if err != nil {
				firstErr = err
				cancel()
				break
			}
			results[folded] = result
			folded++
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	if folded < len(jobs) {
		// The caller's context ended before every job was handed out.
		return nil, fmt.Errorf("aicert: %d of %d runs not scored: %w", len(jobs)-folded, len(jobs), context.Cause(ctx))
	}
	return results, nil
}
