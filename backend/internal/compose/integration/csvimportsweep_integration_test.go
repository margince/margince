// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/migration"
)

// backdateImportRun makes a run look as if its process stopped writing to it
// past the commit budget.
func backdateImportRun(t *testing.T, e *apptest.AppEnv, runID string) {
	t.Helper()
	if _, err := e.Owner.Exec(t.Context(),
		`UPDATE import_run SET updated_at = now() - interval '31 minutes' WHERE id = $1`, runID); err != nil {
		t.Fatalf("backdating the run: %v", err)
	}
}

func sweepImportRuns(t *testing.T, e *apptest.AppEnv) {
	t.Helper()
	if err := compose.DriveImportRunSweepForTest(context.Background(), e.Pool); err != nil {
		t.Fatalf("one sweep pass: %v", err)
	}
}

func TestALegacyRunningRunIsNeverRequeued(t *testing.T) {
	e := setupImportApp(t)
	commit := compose.ImportCommitArgs{}.Kind()
	runID := approvedButUnworked(t, e)
	killJobs(t, e, commit, runID)
	claimThenFallSilent(t, e, runID, 0)
	// Old code approved a run a new worker had validated, so the status it
	// entered is not the one its attempt was queued for.
	if _, err := e.Owner.Exec(t.Context(),
		`UPDATE import_run SET job_status = 'validating' WHERE id = $1`, runID); err != nil {
		t.Fatalf("making the run legacy: %v", err)
	}
	legacy := readLease(t, e, runID)

	sweepImportRuns(t, e)
	if polled := readRun(t, e, runID); polled.Status != "running" {
		t.Fatalf("a legacy run inside its idle window was swept: %q", polled.Status)
	}
	if got := readLease(t, e, runID); got.attempt != legacy.attempt || got.resumes != 0 {
		t.Fatalf("the sweep moved a legacy run to attempt %d with %d resumes; it must never re-queue one",
			got.attempt, got.resumes)
	}

	backdateImportRun(t, e, runID)
	sweepImportRuns(t, e)
	if polled := readRun(t, e, runID); polled.Status != "failed" || !strings.Contains(polled.Error, "server restarted") {
		t.Fatalf("after the sweep: %q %q, want failed saying the server restarted", polled.Status, polled.Error)
	}
	if jobs := jobsOf(t, e, commit, runID); !slices.Equal(jobs, []attemptJob{{legacy.attempt, "running"}}) {
		t.Fatalf("the legacy run's jobs are %v, want no new one", jobs)
	}

	approveAndRead(t, e, runID)
	if n := leadCount(t, e); n != 3 {
		t.Fatalf("%d leads after the resumed commit, want 3", n)
	}
}

func TestTheSweepLeavesAnUndoingRunResumable(t *testing.T) {
	e := setupImportApp(t)
	profile, _ := uploadCSV(t, e, "lead", prospectCSV)
	run, _ := createRunWithMapping(t, e, "lead", profile.SourceRef, profile.SuggestedMapping)
	approveAndRead(t, e, run.ID)

	if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/undo", nil, nil, nil); status != http.StatusAccepted {
		t.Fatalf("undo → %d, want 202", status)
	}
	backdateImportRun(t, e, run.ID)
	sweepImportRuns(t, e)
	if polled := readRun(t, e, run.ID); polled.Status != "undoing" {
		t.Fatalf("the sweep moved an undoing run to %q; it must stay resumable", polled.Status)
	}
}

func TestAStaleLegacyTestRunReturnsToDraft(t *testing.T) {
	e := setupImportApp(t)
	profile, _ := uploadCSV(t, e, "lead", prospectCSV)
	run, _ := createRunWithMapping(t, e, "lead", profile.SourceRef, profile.SuggestedMapping)
	if _, err := e.Owner.Exec(t.Context(),
		`UPDATE import_run SET status = 'validating', job_attempt = 0, updated_at = now() WHERE id = $1`, run.ID); err != nil {
		t.Fatalf("putting the run back in validating: %v", err)
	}

	sweepImportRuns(t, e)
	if polled := readRun(t, e, run.ID); polled.Status != "validating" {
		t.Fatalf("a test run its request may still be working was swept: %q", polled.Status)
	}

	backdateImportRun(t, e, run.ID)
	sweepImportRuns(t, e)
	if polled := readRun(t, e, run.ID); polled.Status != "pending" || polled.Error != migration.CauseTestRunStopped {
		t.Fatalf("after the sweep: %q %q, want a draft saying the test run stopped", polled.Status, polled.Error)
	}
}

func TestALegacyCompleteRunReleasesItsSource(t *testing.T) {
	e := setupImportApp(t)
	profile, _ := uploadCSV(t, e, "lead", prospectCSV)
	run, _ := createRunWithMapping(t, e, "lead", profile.SourceRef, profile.SuggestedMapping)
	approveAndRead(t, e, run.ID)
	// A legacy finished run still holds its file, and the reaper long since
	// retired the upload's intent because the run adopted the key.
	if _, err := e.Owner.Exec(t.Context(), `
		WITH held AS (UPDATE import_run SET source_released_at = NULL WHERE id = $1 RETURNING source_ref)
		DELETE FROM stored_object_intent WHERE storage_key = (SELECT source_ref FROM held)`, run.ID); err != nil {
		t.Fatalf("holding the finished run's source: %v", err)
	}

	sweepImportRuns(t, e)
	var released bool
	var intents int
	if err := e.Owner.QueryRow(t.Context(), `
		SELECT r.source_released_at IS NOT NULL AND r.profile IS NULL,
		       (SELECT count(*) FROM stored_object_intent i WHERE i.storage_key = r.source_ref AND i.kind = 'import')
		  FROM import_run r WHERE r.id = $1`, run.ID).Scan(&released, &intents); err != nil {
		t.Fatalf("reading the run's release: %v", err)
	}
	if !released || intents != 1 {
		t.Fatalf("after the sweep the run is released %v with %d reaper intent(s), want released with one", released, intents)
	}
	if polled := readRun(t, e, run.ID); polled.Status != "complete" {
		t.Fatalf("releasing the source moved the run to %q, want it complete", polled.Status)
	}
	if status := importCall(t, e, run.ID, "undo", nil, nil); status != http.StatusAccepted {
		t.Fatalf("undo of a released run → %d, want 202: undo reads the map, never the file", status)
	}
	if polled := readRun(t, e, run.ID); polled.Status != "undone" {
		t.Fatalf("the undo of a released run ended %q, want undone", polled.Status)
	}
}
