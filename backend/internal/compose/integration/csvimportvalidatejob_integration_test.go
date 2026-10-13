// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A test run is queued in the transaction that moves the run to validating,
// and the request answers. The worker reads the file in one snapshot and
// writes its progress beside it. It parks the run with its report, or returns
// it to its draft with the reason.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/compose/integration/importjobtest"
	"github.com/margince/margince/backend/internal/compose/integration/jobtest"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
	"github.com/margince/margince/backend/internal/platform/jobs"
)

// queueLeadTest uploads a lead file and starts its test run, which it leaves
// queued.
func queueLeadTest(t *testing.T, e *apptest.AppEnv, csv string) importRunDTO {
	t.Helper()
	profile, _ := uploadCSV(t, e, "lead", csv)
	var queued importRunDTO
	if status := e.Call(t, http.MethodPost, "/v1/imports", AnyMap{
		"connector": "csv", "object": "lead", "source_ref": profile.SourceRef,
		"mapping": map[string]string{"Email": "email", "Full Name": "full_name"},
	}, nil, &queued); status != http.StatusAccepted || queued.Status != "validating" {
		t.Fatalf("starting the test run → %d %q, want 202 validating", status, queued.Status)
	}
	return queued
}

// testRunState is what a second connection reads of a run while its test runs.
type testRunState struct {
	status, phase string
	progress      migration.Progress
}

func readTestRunState(t *testing.T, e *apptest.AppEnv, id string) testRunState {
	t.Helper()
	var state testRunState
	var phase *string
	var progress []byte
	if err := e.Owner.QueryRow(t.Context(), `SELECT status, phase, progress FROM import_run WHERE id = $1`, id).
		Scan(&state.status, &phase, &progress); err != nil {
		t.Fatalf("reading the run: %v", err)
	}
	if phase != nil {
		state.phase = *phase
	}
	if len(progress) > 0 {
		if err := json.Unmarshal(progress, &state.progress); err != nil {
			t.Fatalf("decoding the run's progress %s: %v", progress, err)
		}
	}
	return state
}

func TestATestRunAnswersBeforeAnyRowIsRead(t *testing.T) {
	e := setupImportApp(t)
	queued := queueLeadTest(t, e, prospectCSV)
	if state := readTestRunState(t, e, queued.ID); state.phase != "" || len(state.progress.Objects) != 0 {
		t.Fatalf("the run reads %+v when the request answers, want no phase and no progress: the worker reads the rows", state)
	}
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+queued.ID+"/report", nil, nil, nil); status != http.StatusConflict {
		t.Fatalf("the report while the test is queued → %d, want 409", status)
	}
	var again importRunDTO
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+queued.ID+"/validate", AnyMap{"version": queued.Version}, nil, &again); status != http.StatusOK || again.Status != "validating" {
		t.Fatalf("a second test of the same version → %d %q, want 200 with the run still validating", status, again.Status)
	}
	jobs := importjobtest.Queued(t, e, compose.ImportValidateArgs{}.Kind())
	attempt, _ := runLease(t, e, queued.ID)
	if len(jobs) != 1 || !jobNames(t, jobs[0], queued.ID, attempt) {
		t.Fatalf("queued %d test jobs, want one naming run %s at attempt %d", len(jobs), queued.ID, attempt)
	}

	results := importjobtest.WorkQueued(t, e, compose.ImportValidateArgs{}.Kind())
	if len(results) != 1 || results[0].EventKind != river.EventKindJobCompleted {
		t.Fatalf("worked %d test jobs, want the one queued, completed", len(results))
	}
	if tested := readRun(t, e, queued.ID); tested.Status != "awaiting_approval" || readImportReport(t, e, queued.ID).RowsRead != 3 {
		t.Fatalf("after the test job the run is %q, want awaiting approval with the 3 rows read", tested.Status)
	}
}

// jobNames answers whether a test job's args name the run at attempt.
func jobNames(t *testing.T, job *rivertype.JobRow, id string, attempt int) bool {
	t.Helper()
	var args compose.ImportValidateArgs
	if err := json.Unmarshal(job.EncodedArgs, &args); err != nil {
		t.Fatalf("decoding the test job's args: %v", err)
	}
	return args.RunID.String() == id && args.Attempt == attempt
}

func TestAnOverBudgetTestRunReturnsToDraft(t *testing.T) {
	e := setupImportApp(t)
	queued := queueLeadTest(t, e, leadFile(12_000))

	// The job's context ends long before 12,000 rows can be read twice.
	results := importjobtest.WorkQueuedBy(t, e, e.ImportJobs(), compose.ImportValidateArgs{}.Kind(), 50*time.Millisecond)
	if len(results) != 1 || results[0].EventKind != river.EventKindJobCompleted {
		t.Fatalf("the timed-out test ended %+v, want its job completed, not snoozed or retried", results)
	}
	if run := readRun(t, e, queued.ID); run.Status != "pending" || run.Error != migration.CauseValidateOverBudget {
		t.Fatalf("the run is %q %q, want back at its draft with the over-budget reason", run.Status, run.Error)
	}
	var claimed bool
	if err := e.Owner.QueryRow(t.Context(), `SELECT started_at IS NOT NULL FROM import_run WHERE id = $1`, queued.ID).Scan(&claimed); err != nil || !claimed {
		t.Fatalf("the test was never claimed (%v), so its deadline ended it before any row was read", err)
	}
	if left := importjobtest.Queued(t, e, importjobtest.Kinds...); len(left) != 0 {
		t.Fatalf("%d import jobs still queued after a timed-out test, want none", len(left))
	}
}

func TestAWorkerWithNoObjectStoreFailsTheValidation(t *testing.T) {
	e := setupImportApp(t)
	queued := queueLeadTest(t, e, prospectCSV)

	storeless, err := compose.ImportJobWorkersForTest(e.Pool, compose.JobRunnerConfig{})
	if err != nil {
		t.Fatalf("building the workers of a role with no object store: %v", err)
	}
	results := importjobtest.WorkQueuedBy(t, e, storeless, compose.ImportValidateArgs{}.Kind(), 0)
	if len(results) != 1 || results[0].EventKind != river.EventKindJobCompleted {
		t.Fatalf("the test ended %+v, want its job completed with the run returned to its draft", results)
	}
	if run := readRun(t, e, queued.ID); run.Status != "pending" || run.Error != migration.CauseTestStorageUnavailable {
		t.Fatalf("the run is %q %q, want back at its draft for want of an object store", run.Status, run.Error)
	}
}

// testRunnerApp is an app and a running worker over one object store, so a
// test run is worked as the worker role works it.
func testRunnerApp(t *testing.T) (*apptest.AppEnv, *jobs.Runner) {
	t.Helper()
	store := blobstore.NewMemory()
	e := apptest.SetupAppWithOptions(t, compose.WithBlobstore(store))
	e.BootstrapWorkspace(t)
	runner, _, _ := jobtest.StartTestJobRunner(t, e.Pool, compose.JobRunnerConfig{
		Blobstore: store, ImportUploadCap: deployconfig.Config{}.EffectiveUploads().CSVImport,
	})
	return e, runner
}

// awaitMidPart polls the run from its own connection until its progress shows
// a part read partway, and answers what it read. The runner offers no
// subscription to a flush, so the run row is read.
func awaitMidPart(t *testing.T, e *apptest.AppEnv, id string) testRunState {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		state := readTestRunState(t, e, id)
		for _, object := range state.progress.Objects {
			if state.status == "validating" && object.RowsDone > 0 && object.RowsDone < object.RowsTotal {
				return state
			}
		}
		if state.status != "validating" {
			t.Fatalf("the test run ended %q before its progress showed a part read partway; the file is too small to see it move", state.status)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("run %s showed no progress partway through a part: %v", id, ctx.Err())
		case <-tick.C:
		}
	}
}

func TestTestRunProgressIsReadableWhileItRuns(t *testing.T) {
	e, runner := testRunnerApp(t)
	const rows = 12_000
	queued := queueLeadTest(t, e, leadFile(rows))

	mid := awaitMidPart(t, e, queued.ID)
	if mid.phase != migration.PhaseReading && mid.phase != migration.PhaseChecking {
		t.Errorf("mid-read the phase is %q, want reading or checking", mid.phase)
	}
	if lead := mid.progress.Objects[0]; lead.Object != migration.ObjectLead || lead.RowsTotal != rows {
		t.Errorf("mid-read the progress names %+v, want the lead file's %d rows", lead, rows)
	}
	awaitStatus(t, e, queued.ID, "awaiting_approval")
	stopRunner(t, runner)
	if done := readTestRunState(t, e, queued.ID); done.phase != "" || len(done.progress.Objects) != 0 {
		t.Fatalf("the tested run still reads %+v, want its read's progress cleared", done)
	}
}

func TestAStoppedTestRunRestartsFromZero(t *testing.T) {
	e, runner := testRunnerApp(t)
	const rows = 12_000
	queued := queueLeadTest(t, e, leadFile(rows))
	attempt, _ := runLease(t, e, queued.ID)

	awaitMidPart(t, e, queued.ID)
	stopRunner(t, runner)

	if stopped := readRun(t, e, queued.ID); stopped.Status != "validating" || stopped.Error != "" {
		t.Fatalf("after the shutdown the run is %q %q, want still validating with no error", stopped.Status, stopped.Error)
	}
	if now, claimed := runLease(t, e, queued.ID); now != attempt || claimed != nil {
		t.Fatalf("the stopped test left attempt %d claimed by %v, want attempt %d queued again", now, claimed, attempt)
	}
	var state rivertype.JobState
	if err := e.Owner.QueryRow(t.Context(), `SELECT state FROM river_job WHERE kind = $1 AND args->>'run_id' = $2`,
		compose.ImportValidateArgs{}.Kind(), queued.ID).Scan(&state); err != nil {
		t.Fatalf("reading the test job: %v", err)
	}
	if state != rivertype.JobStateAvailable && state != rivertype.JobStateScheduled {
		t.Fatalf("the test job is %s after the shutdown, want it waiting for the next claim", state)
	}

	importjobtest.Drain(t, e)
	tested := readRun(t, e, queued.ID)
	if report := readImportReport(t, e, queued.ID); tested.Status != "awaiting_approval" || report.RowsRead != rows ||
		report.Disposition.Created != rows {
		t.Fatalf("the restarted test ended %q reading %d rows and %+v, want every one of %d read from the top",
			tested.Status, report.RowsRead, report.Disposition, rows)
	}
}

// awaitStatus polls the run until it reaches status.
func awaitStatus(t *testing.T, e *apptest.AppEnv, id, status string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for readRun(t, e, id).Status != status {
		select {
		case <-ctx.Done():
			t.Fatalf("run %s never reached %s: %v", id, status, ctx.Err())
		case <-tick.C:
		}
	}
}

func stopRunner(t *testing.T, runner *jobs.Runner) {
	t.Helper()
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 30*time.Second)
	defer cancel()
	if err := runner.StopAndCancel(stopCtx); err != nil {
		t.Fatalf("StopAndCancel: %v", err)
	}
}
