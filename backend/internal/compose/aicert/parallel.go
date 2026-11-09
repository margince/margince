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
// A failure stops new jobs from starting and lets running ones finish. Every
// job before the earliest failure is folded, and that failure is returned, as
// one worker scoring the jobs in order would leave it.
func scoreConcurrently[R any](ctx context.Context, jobs []runJob, workers int,
	measure func(ctx context.Context, worker int, job runJob) (R, error),
	fold func(job runJob, measured R) (RunResult, error),
) ([]RunResult, error) {
	done, stop := dispatch(ctx, jobs, workers, measure)
	results := make([]RunResult, len(jobs))
	waiting := map[int]R{}
	failures := map[int]error{}
	failedAt, folded := len(jobs), 0
	for f := range done {
		if f.err != nil {
			failures[f.at] = f.err
			failedAt = min(failedAt, f.at)
			stop()
			continue
		}
		waiting[f.at] = f.measured
		for folded < failedAt {
			measured, ready := waiting[folded]
			if !ready {
				break
			}
			delete(waiting, folded)
			result, err := fold(jobs[folded], measured)
			if err != nil {
				failures[folded], failedAt = err, folded
				stop()
				break
			}
			results[folded] = result
			folded++
		}
	}
	if failedAt < len(jobs) {
		return nil, failures[failedAt]
	}
	if folded < len(jobs) {
		// The caller's context ended before every job was handed out.
		return nil, fmt.Errorf("aicert: %d of %d runs not scored: %w", len(jobs)-folded, len(jobs), context.Cause(ctx))
	}
	return results, nil
}

// measuredJob is one job's measurement, or the error it failed with.
type measuredJob[R any] struct {
	at       int
	measured R
	err      error
}

// dispatch hands jobs to workers in job order and streams what each measured.
// The channel closes once every started job is done; stop hands out no more.
func dispatch[R any](ctx context.Context, jobs []runJob, workers int,
	measure func(ctx context.Context, worker int, job runJob) (R, error),
) (<-chan measuredJob[R], func()) {
	next := make(chan int)
	done := make(chan measuredJob[R])
	halt := make(chan struct{})
	var wg sync.WaitGroup
	for worker := range min(workers, len(jobs)) {
		wg.Go(func() {
			for at := range next {
				measured, err := measure(ctx, worker, jobs[at])
				done <- measuredJob[R]{at: at, measured: measured, err: err}
			}
		})
	}
	go func() {
		defer close(next)
		for at := range jobs {
			select {
			case <-halt:
				return // checked first, since a ready send would otherwise race it
			default:
			}
			select {
			case next <- at:
			case <-halt:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(done)
	}()
	return done, sync.OnceFunc(func() { close(halt) })
}
