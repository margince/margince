// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/buildinfo"
)

// dataTaskLongestWait is how long execute waits on a step's retry before it
// stops and reports the step. A longer wait is a step in backoff after
// failures, which a human should read rather than outwait.
const dataTaskLongestWait = 30 * time.Second

// dataTaskLeftoverCap bounds the recount of each finished step's scope.
const dataTaskLeftoverCap = 10000

// DataTaskRun is what one execute leaves: the ledger's progress, the rows each
// finished step still counts in scope, and the version each task needs.
type DataTaskRun struct {
	Progress    []datatask.Progress
	LeftInScope map[string]int64
	Required    map[string]int
}

// DataTaskExecute runs the release's corrections in this process, through the
// same engine and catalog the worker runs. It starts no job queue and no
// event lane, so nothing reaches a mailbox, a model or an enrichment vendor.
// Events the corrections write wait in the outbox for the next worker.
func DataTaskExecute(ctx context.Context, pool *pgxpool.Pool, maxBatches int) (DataTaskRun, error) {
	ctx, engine, err := dataTaskJobEngine(ctx, pool)
	if err != nil {
		return DataTaskRun{}, err
	}
	if err := engine.Seed(ctx); err != nil {
		return DataTaskRun{}, err
	}
	progress, err := executeDataTasks(ctx, engine, maxBatches, sleepFor)
	if err != nil {
		return DataTaskRun{}, err
	}
	return recountDataTasks(ctx, pool, dataTaskCatalog(pool, time.Now), progress)
}

// recountDataTasks asks each finished step's own estimate what is still in its
// scope. A row a step saw and left, such as mail with no stored original,
// still counts there.
func recountDataTasks(ctx context.Context, pool *pgxpool.Pool, catalog []datatask.Task, progress []datatask.Progress) (DataTaskRun, error) {
	run := DataTaskRun{Progress: progress, LeftInScope: map[string]int64{}, Required: map[string]int{}}
	done := map[string]bool{}
	for _, p := range progress {
		done[p.TaskID+"/"+p.Step] = p.Status == datatask.Done && p.BlockedReason == ""
	}
	for _, task := range catalog {
		run.Required[task.ID] = task.Version
		for _, step := range task.Steps {
			name := task.ID + "/" + step.Name
			if !done[name] {
				continue
			}
			var left int64
			err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
				var err error
				left, _, err = step.Estimate(ctx, tx, dataTaskLeftoverCap)
				return err
			})
			if err != nil {
				return DataTaskRun{}, fmt.Errorf("recounting %s: %w", name, err)
			}
			if left > 0 {
				run.LeftInScope[name] = left
			}
		}
	}
	return run, nil
}

// dataTaskIdleLimit bounds the attempts in a row that commit nothing. Such a
// step is busy or waiting to retry, and the bound lets a stuck step end the run.
const dataTaskIdleLimit = 60

// executeDataTasks commits up to maxBatches batches. It stops early when no
// step has more to do, or a step waits longer than dataTaskLongestWait. It also
// stops after dataTaskIdleLimit attempts in a row that commit nothing.
func executeDataTasks(
	ctx context.Context, engine *datatask.Engine, maxBatches int, sleep func(context.Context, time.Duration) error,
) ([]datatask.Progress, error) {
	for committed, idle := 0, 0; committed < maxBatches && idle < dataTaskIdleLimit; {
		outcome, err := engine.Execute(ctx)
		if err != nil {
			return nil, err
		}
		wait := outcome.Wait
		if outcome.Busy {
			wait = time.Second
		}
		if wait == 0 {
			committed, idle = committed+1, 0
		} else {
			idle++
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
// that did not. It does not say the run was accepted. A finished step that
// left rows in its scope, or inspected nothing, is named as well.
type DataTaskVerdict struct {
	Release          string           `json:"release"`
	Finished         bool             `json:"finished"`
	Done             int              `json:"done"`
	Required         map[string]int   `json:"required_versions"`
	Unfinished       []string         `json:"unfinished,omitempty"`
	AwaitingReview   []string         `json:"awaiting_review,omitempty"`
	Blocked          []string         `json:"blocked,omitempty"`
	Failed           []string         `json:"failed,omitempty"`
	LeftInScope      map[string]int64 `json:"left_in_scope,omitempty"`
	NothingInspected []string         `json:"nothing_inspected,omitempty"`
}

// DataTaskVerdictOf reads the verdict off one run.
func DataTaskVerdictOf(run DataTaskRun) DataTaskVerdict {
	v := DataTaskVerdict{Release: buildinfo.ReleaseVersion, Required: run.Required, LeftInScope: run.LeftInScope}
	for _, p := range run.Progress {
		step := p.TaskID + "/" + p.Step
		if needs, ok := run.Required[p.TaskID]; ok && p.TaskVersion < needs {
			step += fmt.Sprintf(" (stored v%d, needs v%d)", p.TaskVersion, needs)
		}
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
