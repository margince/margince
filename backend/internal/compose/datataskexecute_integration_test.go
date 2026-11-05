// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// data-tasks execute runs corrections in this process. These tests drive it
// over synthetic corrections, so each outcome the verdict names is reached.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func probeStep(name string, batch func(context.Context, pgx.Tx, datatask.BatchIn) (datatask.BatchOut, error)) datatask.Step {
	return datatask.Step{
		Name: name, Limit: 4, MinLimit: 4, Audit: datatask.AuditPolicy{PerRecord: true},
		Estimate: func(context.Context, datatask.Querier, int64) (int64, bool, error) { return 0, false, nil },
		Batch:    batch,
	}
}

// probeBatch writes one system_log row per batch and finishes on its second.
func probeBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if _, err := storekit.LogSystem(ctx, tx, "data_task.execute_probe", map[string]any{"run": in.RunID}); err != nil {
		return datatask.BatchOut{}, err
	}
	if len(in.Cursor) == 0 {
		return datatask.BatchOut{Cursor: json.RawMessage(`"1"`), Scanned: 1, Changed: 1}, nil
	}
	return datatask.BatchOut{Done: true, Scanned: 1, Changed: 1}, nil
}

func probeEngine(t *testing.T, e *integration.Env, now *time.Time, tasks ...datatask.Task) (context.Context, *datatask.Engine) {
	t.Helper()
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	engine, err := datatask.New(e.Pool, tasks, "test", func() time.Time { return *now },
		func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	return ctx, engine
}

func TestExecuteFinishesAutomaticWorkAndNamesWhatItCouldNotFinish(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	probe := datatask.Task{ID: "core/100_probe", Version: 1, Title: "Probe", Steps: []datatask.Step{probeStep("write", probeBatch)}}
	empty := datatask.Task{
		ID: "core/200_empty", Version: 1, Title: "Empty",
		Steps: []datatask.Step{probeStep("scan", func(context.Context, pgx.Tx, datatask.BatchIn) (datatask.BatchOut, error) {
			return datatask.BatchOut{Done: true}, nil
		})},
	}
	manual := datatask.Task{
		ID: "core/300_manual", Version: 1, Title: "Manual", Manual: true, ReviewVersion: "rules-1",
		Review: func(context.Context, pgx.Tx, datatask.ReviewRequest) (string, error) { return "", nil },
		Steps:  []datatask.Step{probeStep("apply", probeBatch)},
	}
	ctx, engine := probeEngine(t, e, &now, probe, empty, manual)
	before := e.WsCount(t, `SELECT count(*) FROM river_job`)

	progress, err := executeDataTasks(ctx, engine, 100, func(context.Context, time.Duration) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	verdict := DataTaskVerdictOf(DataTaskRun{Progress: progress})
	if verdict.Finished || verdict.Done != 2 || !slices.Equal(verdict.AwaitingReview, []string{"core/300_manual/apply"}) {
		t.Errorf("verdict = %+v, want two steps done and the manual one awaiting review", verdict)
	}
	if !slices.Equal(verdict.NothingInspected, []string{"core/200_empty/scan"}) {
		t.Errorf("nothing inspected = %v, want the empty step named", verdict.NothingInspected)
	}
	probes := `SELECT count(*) FROM system_log WHERE action = 'data_task.execute_probe'`
	if n := e.WsCount(t, probes); n != 2 {
		t.Errorf("%d probe batches committed, want 2; the manual step must not run unreviewed", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM river_job`); n != before {
		t.Errorf("execute queued %d jobs; it runs in this process", n-before)
	}

	// A second run finds everything done and changes nothing.
	if _, err := executeDataTasks(ctx, engine, 100, func(context.Context, time.Duration) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if n := e.WsCount(t, probes); n != 2 {
		t.Errorf("a second run committed %d more probe batches", n-2)
	}
}

func TestExecuteStopsAtAFailedStepAndNamesIt(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	failing := datatask.Task{
		ID: "core/100_failing", Version: 1, Title: "Failing",
		Steps: []datatask.Step{probeStep("write", func(context.Context, pgx.Tx, datatask.BatchIn) (datatask.BatchOut, error) {
			return datatask.BatchOut{}, errors.New("synthetic failure")
		})},
	}
	later := datatask.Task{ID: "core/200_later", Version: 1, Title: "Later", Steps: []datatask.Step{probeStep("write", probeBatch)}}
	ctx, engine := probeEngine(t, e, &now, failing, later)
	progress, err := executeDataTasks(ctx, engine, 50, func(context.Context, time.Duration) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	// The first failure waits longer than execute outwaits, so it stops and
	// names the step waiting to retry, and the step held behind it.
	verdict := DataTaskVerdictOf(DataTaskRun{Progress: progress})
	want := []string{"core/100_failing/write (retry_wait)", "core/200_later/write (earlier_step)"}
	if verdict.Finished || !slices.Equal(verdict.Blocked, want) {
		t.Errorf("verdict = %+v, want blocked %v", verdict, want)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM system_log WHERE action = 'data_task.execute_probe'`); n != 0 {
		t.Errorf("the later step ran %d batches past a failed one", n)
	}
}

// trapTransport fails every outbound request and counts it.
type trapTransport struct{ calls *atomic.Int64 }

func (t trapTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.calls.Add(1)
	return nil, errors.New("data-tasks execute made an outbound request")
}

// The release's whole catalog, through the command's own entry point. Nothing
// leaves the database: no request, no queued job, no model call.
func TestExecuteRunsTheReleaseCatalogWithoutReachingOutside(t *testing.T) {
	e := integration.Setup(t)
	calls := &atomic.Int64{}
	saved := http.DefaultTransport
	http.DefaultTransport = trapTransport{calls: calls}
	t.Cleanup(func() { http.DefaultTransport = saved })
	jobs := e.WsCount(t, `SELECT count(*) FROM river_job`)
	models := e.WsCount(t, `SELECT count(*) FROM ai_call`)

	run, err := DataTaskExecute(e.Admin(), e.Pool, 10000)
	if err != nil {
		t.Fatal(err)
	}
	catalog := dataTaskCatalog(e.Pool, func() time.Time { return clocktest.Now(t) })
	if len(run.Progress) < len(catalog) || len(run.Required) != len(catalog) {
		t.Errorf("the run reports %d steps and %d required versions for %d tasks", len(run.Progress), len(run.Required), len(catalog))
	}
	if n := calls.Load(); n != 0 {
		t.Errorf("the run made %d outbound requests", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM river_job`); n != jobs {
		t.Errorf("the run queued %d jobs", n-jobs)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM ai_call`); n != models {
		t.Errorf("the run recorded %d model calls", n-models)
	}
	verdict := DataTaskVerdictOf(run)
	if verdict.Done == 0 || len(verdict.Failed) != 0 {
		t.Errorf("verdict = %+v, want automatic steps done and none failed", verdict)
	}
}

// Only a step whose estimate counts rows still to correct is recounted. Its
// leftover rows are named. A step whose estimate counts input it keeps, and a
// finished row from an older version, are not recounted.
func TestTheRecountNamesRowsAStepLeftAndNothingElse(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	counting := func(n int64) func(context.Context, datatask.Querier, int64) (int64, bool, error) {
		return func(context.Context, datatask.Querier, int64) (int64, bool, error) { return n, false, nil }
	}
	leftover := probeStep("assess", probeBatch)
	leftover.Estimate, leftover.EmptyWhenDone = counting(3), true
	kept := probeStep("count", probeBatch)
	kept.Estimate = counting(7)
	stale := probeStep("assess", probeBatch)
	stale.Estimate, stale.EmptyWhenDone = counting(9), true
	catalog := []datatask.Task{
		{ID: "core/1_left", Version: 1, Steps: []datatask.Step{leftover}},
		{ID: "core/2_kept", Version: 1, Steps: []datatask.Step{kept}},
		{ID: "core/3_stale", Version: 2, Steps: []datatask.Step{stale}},
	}
	run, err := recountDataTasks(ctx, e.Pool, catalog, []datatask.Progress{
		{TaskID: "core/1_left", Step: "assess", Status: datatask.Done, TaskVersion: 1},
		{TaskID: "core/2_kept", Step: "count", Status: datatask.Done, TaskVersion: 1},
		{TaskID: "core/3_stale", Step: "assess", Status: datatask.Done, TaskVersion: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(run.LeftInScope) != 1 || run.LeftInScope["core/1_left/assess"] != 3 {
		t.Errorf("left in scope = %v, want only the marked step's three rows", run.LeftInScope)
	}
	verdict := DataTaskVerdictOf(run)
	if verdict.Finished || !slices.Equal(verdict.Unfinished, []string{"core/3_stale/assess (stored v1, needs v2)"}) {
		t.Errorf("verdict = %+v, want the stale step unfinished", verdict)
	}
}
