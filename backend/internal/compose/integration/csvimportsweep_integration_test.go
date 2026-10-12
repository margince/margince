// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
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
	if err := compose.DriveImportRunSweepForTest(context.Background(), e.Pool, time.Now); err != nil {
		t.Fatalf("one sweep pass: %v", err)
	}
}

func TestTheSweepFailsARunWhoseProcessStoppedAndApproveResumesIt(t *testing.T) {
	e, runner := setupImportAppHeld(t)
	profile, _ := uploadCSV(t, e, "lead", prospectCSV)
	run, _ := createRunWithMapping(t, e, "lead", profile.SourceRef, profile.SuggestedMapping)
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, nil); status != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", status)
	}

	sweepImportRuns(t, e)
	if polled := readRun(t, e, run.ID); polled.Status != "running" {
		t.Fatalf("a run inside its budget was swept: %q", polled.Status)
	}

	backdateImportRun(t, e, run.ID)
	sweepImportRuns(t, e)
	var failed struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	e.Call(t, http.MethodGet, "/v1/imports/"+run.ID, nil, nil, &failed)
	if failed.Status != "failed" || !strings.Contains(failed.Error, "server restarted") {
		t.Fatalf("after the sweep: %q %q, want failed saying the server restarted", failed.Status, failed.Error)
	}

	runner.inline = true
	approveAndRead(t, e, run.ID)
	if n := leadCount(t, e); n != 3 {
		t.Fatalf("%d leads after the resumed commit, want 3", n)
	}
}

func TestTheSweepLeavesAnUndoingRunResumable(t *testing.T) {
	e, runner := setupImportAppHeld(t)
	runner.inline = true
	profile, _ := uploadCSV(t, e, "lead", prospectCSV)
	run, _ := createRunWithMapping(t, e, "lead", profile.SourceRef, profile.SuggestedMapping)
	approveAndRead(t, e, run.ID)

	runner.inline = false
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/undo", nil, nil, nil); status != http.StatusAccepted {
		t.Fatalf("undo → %d, want 202", status)
	}
	backdateImportRun(t, e, run.ID)
	sweepImportRuns(t, e)
	if polled := readRun(t, e, run.ID); polled.Status != "undoing" {
		t.Fatalf("the sweep moved an undoing run to %q; it must stay resumable", polled.Status)
	}
}
