// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/blobstore"
)

// heldRunner keeps every detached task until the test releases it, so a test
// sees what the request answered before the work ran.
type heldRunner struct {
	tasks []func()
	// inline runs later tasks at once, once the test is done holding.
	inline bool
}

func (r *heldRunner) run(task func()) {
	if r.inline {
		task()
		return
	}
	r.tasks = append(r.tasks, task)
}

// setupImportAppHeld is setupImportApp with a runner the test controls.
func setupImportAppHeld(t *testing.T) (*apptest.AppEnv, *heldRunner) {
	t.Helper()
	runner := &heldRunner{}
	e := apptest.SetupAppWithOptions(t, compose.WithBlobstore(blobstore.NewMemory()), compose.WithImportCommitRunner(runner.run))
	e.BootstrapWorkspace(t)
	return e, runner
}

func readRun(t *testing.T, e *apptest.AppEnv, id string) importRunDTO {
	t.Helper()
	var run importRunDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+id, nil, nil, &run); status != http.StatusOK {
		t.Fatalf("GET the run → %d, want 200", status)
	}
	return run
}

func TestApprovingAnswersRunningAndThePollSeesItComplete(t *testing.T) {
	e, runner := setupImportAppHeld(t)
	profile, _ := uploadCSV(t, e, "lead", prospectCSV)
	run, _ := createRunWithMapping(t, e, "lead", profile.SourceRef, profile.SuggestedMapping)
	var approved importRunDTO
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, &approved); status != http.StatusAccepted || approved.Status != "running" {
		t.Fatalf("approve → %d %q, want 202 running", status, approved.Status)
	}
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, nil); status != http.StatusConflict {
		t.Fatalf("second approve → %d, want 409 while running", status)
	}
	if len(runner.tasks) != 1 {
		t.Fatalf("%d tasks held, want the one commit", len(runner.tasks))
	}
	runner.tasks[0]()
	if polled := readRun(t, e, run.ID); polled.Status != "complete" || leadCount(t, e) != 3 {
		t.Fatalf("after the commit ran: %q and %d leads", polled.Status, leadCount(t, e))
	}
}

func TestUndoAnswersUndoingAndThePollSeesItUndone(t *testing.T) {
	e, runner := setupImportAppHeld(t)
	runner.inline = true
	profile, _ := uploadCSV(t, e, "lead", prospectCSV)
	run, _ := createRunWithMapping(t, e, "lead", profile.SourceRef, profile.SuggestedMapping)
	approveAndRead(t, e, run.ID)

	runner.inline = false
	var undoing importRunDTO
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/undo", nil, nil, &undoing); status != http.StatusAccepted || undoing.Status != "undoing" {
		t.Fatalf("undo → %d %q, want 202 undoing", status, undoing.Status)
	}
	if leadCount(t, e) != 3 {
		t.Fatal("the undo ran inside the request")
	}
	runner.tasks[0]()
	if polled := readRun(t, e, run.ID); polled.Status != "undone" || leadCount(t, e) != 0 {
		t.Fatalf("after the undo ran: %q and %d leads", polled.Status, leadCount(t, e))
	}
}
