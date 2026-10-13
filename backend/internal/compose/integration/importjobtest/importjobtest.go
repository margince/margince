// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

// Package importjobtest works the import jobs an approve or an undo queued. It
// uses the workers the worker role registers, through River's test executor. The
// River row moves as it would under a runner, so a dedupe and a snooze are real.
//
// It is its own package for the reason jobtest is. It builds on compose and
// apptest, and a non-test file in package integration may not import them.
package importjobtest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertest"
	"github.com/riverqueue/river/rivertype"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// drainPasses bounds Drain. A 500-row slice makes each pass of a test file
// land a whole file, so a run still queued after this many passes is looping.
const drainPasses = 100

// queuedStates are the states a River row waits to be worked in. A snoozed
// slice is scheduled, and the harness does not wait out its delay.
var queuedStates = []rivertype.JobState{rivertype.JobStateAvailable, rivertype.JobStateScheduled}

// Kinds are the import kinds the harness works.
var Kinds = []string{compose.ImportCommitArgs{}.Kind(), compose.ImportUndoArgs{}.Kind()}

// WorkQueued works every queued job of kind once, oldest first, with the
// workers over the server's own object store, and answers how each ended.
func WorkQueued(t *testing.T, e *apptest.AppEnv, kind string) []*rivertest.WorkResult {
	t.Helper()
	return WorkQueuedBy(t, e, e.ImportJobs(), kind, 0)
}

// WorkQueuedBy is WorkQueued with the workers a test chooses, such as a role
// with no object store. A within above zero ends each job's context that soon.
func WorkQueuedBy(
	t *testing.T, e *apptest.AppEnv, workers compose.ImportJobWorkers, kind string, within time.Duration,
) []*rivertest.WorkResult {
	t.Helper()
	var results []*rivertest.WorkResult
	for _, job := range Queued(t, e, kind) {
		results = append(results, work(t, e, workers, job, within))
	}
	return results
}

// Drain works import jobs until none is queued: every slice of every attempt.
func Drain(t *testing.T, e *apptest.AppEnv) {
	t.Helper()
	for range drainPasses {
		jobs := Queued(t, e, Kinds...)
		if len(jobs) == 0 {
			return
		}
		for _, job := range jobs {
			work(t, e, e.ImportJobs(), job, 0)
		}
	}
	t.Fatalf("import jobs are still queued after %d passes", drainPasses)
}

// Queued answers the River rows of kinds still waiting to be worked.
func Queued(t *testing.T, e *apptest.AppEnv, kinds ...string) []*rivertype.JobRow {
	t.Helper()
	listed, err := riverClient(t, e).JobList(t.Context(), river.NewJobListParams().
		Kinds(kinds...).States(queuedStates...).OrderBy(river.JobListOrderByID, river.SortOrderAsc).First(1000))
	if err != nil {
		t.Fatalf("listing the queued import jobs: %v", err)
	}
	return listed.Jobs
}

// work runs one job through the worker of its kind, in a transaction that
// commits River's record of how it ended.
func work(
	t *testing.T, e *apptest.AppEnv, workers compose.ImportJobWorkers, job *rivertype.JobRow, within time.Duration,
) *rivertest.WorkResult {
	t.Helper()
	ctx := t.Context()
	if within > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, within)
		defer cancel()
	}
	tx, err := e.Pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("opening the job's transaction: %v", err)
	}
	defer rollback(t, tx)
	var result *rivertest.WorkResult
	switch job.Kind {
	case compose.ImportCommitArgs{}.Kind():
		result, err = rivertest.NewWorker(t, riverpgxv5.New(e.Pool), riverConfig(), workers.Commit).WorkJob(ctx, t, tx, job)
	case compose.ImportUndoArgs{}.Kind():
		result, err = rivertest.NewWorker(t, riverpgxv5.New(e.Pool), riverConfig(), workers.Undo).WorkJob(ctx, t, tx, job)
	default:
		t.Fatalf("%s is not an import kind", job.Kind)
	}
	if err != nil {
		t.Fatalf("working %s job %d: %v", job.Kind, job.ID, err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("committing %s job %d: %v", job.Kind, job.ID, err)
	}
	return result
}

func rollback(t *testing.T, tx pgx.Tx) {
	t.Helper()
	if err := tx.Rollback(t.Context()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		t.Errorf("rolling back the job's transaction: %v", err)
	}
}

func riverClient(t *testing.T, e *apptest.AppEnv) *river.Client[pgx.Tx] {
	t.Helper()
	client, err := river.NewClient(riverpgxv5.New(e.Pool), riverConfig())
	if err != nil {
		t.Fatalf("river.NewClient: %v", err)
	}
	return client
}

func riverConfig() *river.Config {
	return &river.Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}
