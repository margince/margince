// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/buildinfo"
)

// dataTaskLongestWait is how long execute waits on a step's retry before it
// stops and reports the step. A longer wait is a step in backoff after
// failures, which a human should read rather than outwait.
const dataTaskLongestWait = 30 * time.Second

// DataTaskExecute runs the release's corrections in this process, through the
// same engine and catalog the worker runs. It starts no job queue and no
// event lane, so nothing reaches a mailbox, a model or an enrichment vendor.
// Events the corrections write wait in the outbox for the next worker.
func DataTaskExecute(ctx context.Context, pool *pgxpool.Pool, maxBatches int) ([]datatask.Progress, error) {
	ctx, engine, err := dataTaskJobEngine(ctx, pool)
	if err != nil {
		return nil, err
	}
	if err := engine.Seed(ctx); err != nil {
		return nil, err
	}
	return executeDataTasks(ctx, engine, maxBatches, sleepFor)
}

// executeDataTasks commits batches until no step has more to do, a step waits
// longer than dataTaskLongestWait, or maxBatches have run.
func executeDataTasks(
	ctx context.Context, engine *datatask.Engine, maxBatches int, sleep func(context.Context, time.Duration) error,
) ([]datatask.Progress, error) {
	for range maxBatches {
		outcome, err := engine.Execute(ctx)
		if err != nil {
			return nil, err
		}
		wait := outcome.Wait
		if outcome.Busy {
			wait = time.Second
		}
		if wait > dataTaskLongestWait || (wait == 0 && !outcome.More) {
			break
		}
		if err := sleep(ctx, wait); err != nil {
			return nil, err
		}
	}
	return engine.List(ctx)
}

func sleepFor(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// DataTaskVerdict says whether every correction finished, and names each step
// that did not. It does not say the run was accepted: a step that inspected
// nothing proves nothing about rows it never saw, so those are named too.
type DataTaskVerdict struct {
	Release          string   `json:"release"`
	Finished         bool     `json:"finished"`
	Done             int      `json:"done"`
	Unfinished       []string `json:"unfinished,omitempty"`
	AwaitingReview   []string `json:"awaiting_review,omitempty"`
	Blocked          []string `json:"blocked,omitempty"`
	Failed           []string `json:"failed,omitempty"`
	NothingInspected []string `json:"nothing_inspected,omitempty"`
}

// DataTaskVerdictOf reads the verdict off the ledger's progress.
func DataTaskVerdictOf(progress []datatask.Progress) DataTaskVerdict {
	v := DataTaskVerdict{Release: buildinfo.ReleaseVersion}
	for _, p := range progress {
		step := p.TaskID + "/" + p.Step
		switch {
		case p.AwaitingReview():
			v.AwaitingReview = append(v.AwaitingReview, step)
		case p.Status == datatask.Failed || p.Status == datatask.Paused:
			v.Failed = append(v.Failed, step+" ("+p.Status+")")
		case p.BlockedReason != "":
			v.Blocked = append(v.Blocked, step+" ("+p.BlockedReason+")")
		case p.Status != datatask.Done:
			v.Unfinished = append(v.Unfinished, step)
		default:
			v.Done++
			if p.Scanned == 0 {
				v.NothingInspected = append(v.NothingInspected, step)
			}
		}
	}
	v.Finished = len(v.AwaitingReview)+len(v.Blocked)+len(v.Failed)+len(v.Unfinished) == 0
	return v
}
