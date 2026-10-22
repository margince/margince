// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An approve or an undo moves the run and queues its job in one transaction,
// then answers. The worker writes the rows, slice by slice, and the run row
// says how far it got.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
)

// importCall posts an approve or an undo of a run, then works every job it
// queued. out then holds the run as the worker left it.
//
//craft:ignore naked-any out is whichever shape the caller reads the run into, as e.Call takes it
func importCall(t *testing.T, e *apptest.AppEnv, id, verb string, body, out any) int {
	t.Helper()
	status := e.Call(t, http.MethodPost, "/v1/imports/"+id+"/"+verb, body, nil, out)
	if status != http.StatusAccepted {
		return status
	}
	importjobtest.Drain(t, e)
	if out != nil {
		if got := e.Call(t, http.MethodGet, "/v1/imports/"+id, nil, nil, out); got != http.StatusOK {
			t.Fatalf("GET the run after its %s → %d, want 200", verb, got)
		}
	}
	return status
}

// testCall posts a request that starts a test run, then works the job it
// queued. out then holds the run as the worker left it, or the refusal.
//
//craft:ignore naked-any out is whichever shape the caller reads the run or the refusal into, as e.Call takes it
func testCall(t *testing.T, e *apptest.AppEnv, path string, body, out any) int {
	t.Helper()
	var answer json.RawMessage
	status := e.Call(t, http.MethodPost, path, body, nil, &answer)
	if status == http.StatusAccepted {
		var queued struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal(answer, &queued); err != nil || queued.Status != "validating" {
			t.Fatalf("POST %s → 202 %s (%v), want the run at validating", path, answer, err)
		}
		importjobtest.Drain(t, e)
		if got := e.Call(t, http.MethodGet, "/v1/imports/"+queued.ID, nil, nil, &answer); got != http.StatusOK {
			t.Fatalf("GET the run after its test → %d, want 200", got)
		}
	}
	if out != nil && len(answer) > 0 {
		if err := json.Unmarshal(answer, out); err != nil {
			t.Fatalf("decoding the answer to POST %s: %v", path, err)
		}
	}
	return status
}

func readRun(t *testing.T, e *apptest.AppEnv, id string) importRunDTO {
	t.Helper()
	var run importRunDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+id, nil, nil, &run); status != http.StatusOK {
		t.Fatalf("GET the run → %d, want 200", status)
	}
	return run
}

// leadFile is a lead file of n rows, each a distinct address.
func leadFile(n int) string {
	var b strings.Builder
	b.WriteString("Email,Full Name\n")
	for i := range n {
		fmt.Fprintf(&b, "lead-%05d@slices.example,Lead %05d\n", i, i)
	}
	return b.String()
}

// storedLeads counts every lead row, past the page a list answers.
func storedLeads(t *testing.T, e *apptest.AppEnv) int {
	t.Helper()
	var n int
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM lead`).Scan(&n); err != nil {
		t.Fatalf("counting leads: %v", err)
	}
	return n
}

// testedLeadRun uploads a lead file and test-runs it, ready to approve.
func testedLeadRun(t *testing.T, e *apptest.AppEnv, csv string) importRunDTO {
	t.Helper()
	profile, _ := uploadCSV(t, e, "lead", csv)
	run, status := createRunWithMapping(t, e, "lead", profile.SourceRef, map[string]string{"Email": "email", "Full Name": "full_name"})
	if status != http.StatusAccepted || run.Status != "awaiting_approval" {
		t.Fatalf("test run → %d %q, want 202 awaiting approval", status, run.Status)
	}
	return run
}

// approveQueued approves a run and leaves its commit job queued.
func approveQueued(t *testing.T, e *apptest.AppEnv, id string) {
	t.Helper()
	var approved importRunDTO
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+id+"/approve", nil, nil, &approved); status != http.StatusAccepted || approved.Status != "running" {
		t.Fatalf("approve → %d %q, want 202 running", status, approved.Status)
	}
}

// runLease reads the attempt the run is queued under and the one a slice claimed.
func runLease(t *testing.T, e *apptest.AppEnv, id string) (attempt int, claimed *int) {
	t.Helper()
	if err := e.Owner.QueryRow(t.Context(), `SELECT job_attempt, claimed_attempt FROM import_run WHERE id = $1`, id).Scan(&attempt, &claimed); err != nil {
		t.Fatalf("reading the run's lease: %v", err)
	}
	return attempt, claimed
}

func TestApproveAnswersBeforeAnyRowIsWritten(t *testing.T) {
	e := setupImportApp(t)
	run := testedLeadRun(t, e, prospectCSV)
	approveQueued(t, e, run.ID)
	if n := storedLeads(t, e); n != 0 {
		t.Fatalf("%d leads exist when approve answers, want none: the worker writes them", n)
	}
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, nil); status != http.StatusConflict {
		t.Fatalf("second approve → %d, want 409 while running", status)
	}

	results := importjobtest.WorkQueued(t, e, compose.ImportCommitArgs{}.Kind())
	if len(results) != 1 || results[0].EventKind != river.EventKindJobCompleted {
		t.Fatalf("worked %d commit jobs, want the one the approve queued, completed", len(results))
	}
	if polled := readRun(t, e, run.ID); polled.Status != "complete" || storedLeads(t, e) != 3 {
		t.Fatalf("after the commit job: %q with %d leads, want complete with 3", polled.Status, storedLeads(t, e))
	}
}

func TestUndoAnswersBeforeAnyRowIsReversed(t *testing.T) {
	e := setupImportApp(t)
	run := testedLeadRun(t, e, prospectCSV)
	approveAndRead(t, e, run.ID)

	var undoing importRunDTO
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/undo", nil, nil, &undoing); status != http.StatusAccepted || undoing.Status != "undoing" {
		t.Fatalf("undo → %d %q, want 202 undoing", status, undoing.Status)
	}
	if n := storedLeads(t, e); n != 3 {
		t.Fatalf("%d leads when undo answers, want all 3: the worker reverses them", n)
	}
	importjobtest.WorkQueued(t, e, compose.ImportUndoArgs{}.Kind())
	if polled := readRun(t, e, run.ID); polled.Status != "undone" || leadCount(t, e) != 0 {
		t.Fatalf("after the undo job: %q with %d leads, want undone with none", polled.Status, leadCount(t, e))
	}
}

func TestTagsAreCreatedOnlyByTheCommitJob(t *testing.T) {
	e := setupImportApp(t)
	profile, _ := uploadCSV(t, e, "company", "Name,Tags\nTagged Jobs GmbH,Fresh\n")
	run, _ := createRunWithMapping(t, e, "company", profile.SourceRef, map[string]string{"Name": "display_name", "Tags": "tags"})
	tagsBefore := tagCount(t, e)
	approveQueued(t, e, run.ID)
	if got := tagCount(t, e); got != tagsBefore {
		t.Fatalf("%d tags when approve answers, want %d: the commit job creates them", got, tagsBefore)
	}
	importjobtest.WorkQueued(t, e, compose.ImportCommitArgs{}.Kind())
	if got := tagCount(t, e); got != tagsBefore+1 {
		t.Fatalf("%d tags after the commit job, want %d", got, tagsBefore+1)
	}
}

func TestASnoozedSliceIsClaimedWithTheSameAttempt(t *testing.T) {
	e := setupImportApp(t)
	// One row past a slice, so the first slice snoozes for the second.
	run := testedLeadRun(t, e, leadFile(501))
	approveQueued(t, e, run.ID)
	attempt, _ := runLease(t, e, run.ID)

	first := importjobtest.WorkQueued(t, e, compose.ImportCommitArgs{}.Kind())
	if len(first) != 1 || first[0].EventKind != river.EventKindJobSnoozed {
		t.Fatalf("the first slice ended %+v, want one snoozed job", first)
	}
	// A slow runner ends the slice on its 12 s budget before row 500, so the
	// place is any prefix short of the file.
	if polled := readRun(t, e, run.ID); polled.Status != "running" || polled.Checkpoint < 1 || polled.Checkpoint > 500 {
		t.Fatalf("after one slice the run is %q at %d, want running at 1 to 500", polled.Status, polled.Checkpoint)
	}
	if _, claimed := runLease(t, e, run.ID); claimed != nil {
		t.Fatalf("a snoozed slice left the run claimed by attempt %d, want it queued for the next slice", *claimed)
	}
	checkpoint := readRun(t, e, run.ID).Checkpoint
	for checkpoint < 501 {
		next := importjobtest.WorkQueued(t, e, compose.ImportCommitArgs{}.Kind())
		if len(next) != 1 || next[0].Job.ID != first[0].Job.ID {
			t.Fatalf("the next slice was %+v, want job %d again", next, first[0].Job.ID)
		}
		if next[0].EventKind != river.EventKindJobCompleted && next[0].EventKind != river.EventKindJobSnoozed {
			t.Fatalf("the next slice ended %s, want completion or another bounded slice", next[0].EventKind)
		}
		polled := readRun(t, e, run.ID)
		if polled.Checkpoint <= checkpoint || polled.Checkpoint > 501 {
			t.Fatalf("slice checkpoint moved from %d to %d, want progress within 501 rows", checkpoint, polled.Checkpoint)
		}
		checkpoint = polled.Checkpoint
		sliceAttempt, claimed := runLease(t, e, run.ID)
		if sliceAttempt != attempt || (next[0].EventKind == river.EventKindJobSnoozed && claimed != nil) {
			t.Fatalf("slice attempt %d claimed by %v, want unchanged attempt %d and released snoozes", sliceAttempt, claimed, attempt)
		}
	}
	finalAttempt, claimed := runLease(t, e, run.ID)
	if finalAttempt != attempt || claimed == nil || *claimed != attempt {
		t.Fatalf("the run ended at attempt %d claimed by %v, want attempt %d claimed by it", finalAttempt, claimed, attempt)
	}
	if polled := readRun(t, e, run.ID); polled.Status != "complete" || storedLeads(t, e) != 501 {
		t.Fatalf("the run ended %q with %d leads, want complete with 501", polled.Status, storedLeads(t, e))
	}
}

func TestATimedOutSliceFailsAndDoesNotSnooze(t *testing.T) {
	e := setupImportApp(t)
	run := testedLeadRun(t, e, leadFile(400))
	approveQueued(t, e, run.ID)

	// The slice's job context ends long before 400 row writes can.
	results := importjobtest.WorkQueuedBy(t, e, e.ImportJobs(), compose.ImportCommitArgs{}.Kind(), 50*time.Millisecond)
	if len(results) != 1 || results[0].EventKind != river.EventKindJobCompleted {
		t.Fatalf("the timed-out slice ended %+v, want its job completed, not snoozed", results)
	}
	if polled := readRun(t, e, run.ID); polled.Status != "failed" || polled.Error != migration.CauseOverBudget {
		t.Fatalf("the run is %q %q, want failed over budget", polled.Status, polled.Error)
	}
	if queued := importjobtest.Queued(t, e, importjobtest.Kinds...); len(queued) != 0 {
		t.Fatalf("%d import jobs still queued after a timed-out slice, want none", len(queued))
	}
}

func TestAWorkerWithNoObjectStoreFailsTheRun(t *testing.T) {
	e := setupImportApp(t)
	run := testedLeadRun(t, e, prospectCSV)
	approveQueued(t, e, run.ID)

	storeless, err := compose.ImportJobWorkersForTest(e.Pool, compose.JobRunnerConfig{})
	if err != nil {
		t.Fatalf("building the workers of a role with no object store: %v", err)
	}
	results := importjobtest.WorkQueuedBy(t, e, storeless, compose.ImportCommitArgs{}.Kind(), 0)
	if len(results) != 1 || results[0].EventKind != river.EventKindJobCompleted {
		t.Fatalf("the slice ended %+v, want its job completed with the run failed", results)
	}
	if polled := readRun(t, e, run.ID); polled.Status != "failed" || polled.Error != migration.CauseStorageUnavailable {
		t.Fatalf("the run is %q %q, want failed for want of an object store", polled.Status, polled.Error)
	}
}

func TestAShutdownFlushesAndSnoozesWithoutFailing(t *testing.T) {
	store := blobstore.NewMemory()
	e := apptest.SetupAppWithOptions(t, compose.WithBlobstore(store))
	e.BootstrapWorkspace(t)
	const rows = 1000
	run := testedLeadRun(t, e, leadFile(rows))

	runner, _, _ := jobtest.StartTestJobRunner(t, e.Pool, compose.JobRunnerConfig{
		Blobstore: store, ImportUploadCap: compose.DefaultUploadLimits().CSVImport,
	})
	approveQueued(t, e, run.ID)
	landed := awaitCheckpoint(t, e, run.ID)
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 30*time.Second)
	defer cancel()
	if err := runner.StopAndCancel(stopCtx); err != nil {
		t.Fatalf("StopAndCancel: %v", err)
	}

	stopped := readRun(t, e, run.ID)
	if stopped.Status != "running" || stopped.Error != "" || stopped.Checkpoint < landed {
		t.Fatalf("after the shutdown the run is %q %q at %d, want running with no error at %d or past it",
			stopped.Status, stopped.Error, stopped.Checkpoint, landed)
	}
	if stopped.Checkpoint == rows {
		t.Fatal("the commit finished before the shutdown, so the stop was never tested")
	}
	if _, claimed := runLease(t, e, run.ID); claimed != nil {
		t.Fatalf("the stopped slice left the run claimed by attempt %d, want it queued", *claimed)
	}
	var state rivertype.JobState
	if err := e.Owner.QueryRow(t.Context(), `SELECT state FROM river_job WHERE kind = $1 AND args->>'run_id' = $2`,
		compose.ImportCommitArgs{}.Kind(), run.ID).Scan(&state); err != nil {
		t.Fatalf("reading the commit job: %v", err)
	}
	if state != rivertype.JobStateAvailable && state != rivertype.JobStateScheduled {
		t.Fatalf("the commit job is %s after the shutdown, want it waiting for the next slice", state)
	}

	importjobtest.Drain(t, e)
	if polled := readRun(t, e, run.ID); polled.Status != "complete" || storedLeads(t, e) != rows {
		t.Fatalf("the resumed run ended %q with %d leads, want complete with %d", polled.Status, storedLeads(t, e), rows)
	}
}

// awaitCheckpoint waits for the run's first flush and answers its checkpoint.
// The runner offers no subscription to a snooze, so the run row is read.
func awaitCheckpoint(t *testing.T, e *apptest.AppEnv, id string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		if run := readRun(t, e, id); run.Checkpoint > 0 {
			return run.Checkpoint
		}
		select {
		case <-ctx.Done():
			t.Fatalf("run %s flushed no checkpoint: %v", id, ctx.Err())
		case <-tick.C:
		}
	}
}
