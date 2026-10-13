// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An upload opens one run, and every later choice, test and approval is made
// on it. A second test and "Import anyway" never open another.

import (
	"encoding/json"
	"net/http"
	"slices"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/compose/integration/importjobtest"
)

func readImportRun(t *testing.T, e *apptest.AppEnv, id string) importRunDTO {
	t.Helper()
	var run importRunDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+id, nil, nil, &run); status != http.StatusOK {
		t.Fatalf("GET /v1/imports/%s → %d, want 200", id, status)
	}
	return run
}

func TestCreateImportRunAdvancesTheDraftOfItsSource(t *testing.T) {
	e := setupImportApp(t)
	profile, _ := uploadCSV(t, e, "lead", prospectCSV)
	if profile.RunID == "" || profile.RowCount != 3 {
		t.Fatalf("profile = %+v, want the draft's run_id and a row_count of 3", profile)
	}
	if draft := readImportRun(t, e, profile.RunID); draft.Status != "pending" || draft.Version != 1 {
		t.Fatalf("the upload's run = %+v, want a pending draft at version 1", draft)
	}

	run, status := createRunWithMapping(t, e, "lead", profile.SourceRef, profile.SuggestedMapping)
	if status != http.StatusAccepted || run.ID != profile.RunID || run.Status != "awaiting_approval" {
		t.Fatalf("create → %d %+v, want 202 with the upload's own run awaiting approval", status, run)
	}
	if runs := importRuns(t, e); len(runs) != 1 {
		t.Fatalf("runs = %+v, want the one the upload opened", runs)
	}
}

func TestImportAnywayRetestsTheSameRun(t *testing.T) {
	e := setupImportApp(t)
	profile, _ := uploadCSV(t, e, "lead", prospectCSV)
	tested, _ := createRunWithMapping(t, e, "lead", profile.SourceRef, profile.SuggestedMapping)

	var saved importRunDTO
	if status := e.Call(t, http.MethodPatch, "/v1/imports/"+tested.ID, AnyMap{
		"version": tested.Version, "on_broken_link": "drop_link",
	}, nil, &saved); status != http.StatusOK || saved.Status != "pending" || saved.Version != tested.Version+1 {
		t.Fatalf("PATCH on_broken_link → %d %+v, want 200, back to pending at the next version", status, saved)
	}
	if status := e.Call(t, http.MethodPatch, "/v1/imports/"+tested.ID, AnyMap{
		"version": tested.Version, "on_broken_link": "skip_row",
	}, nil, nil); status != http.StatusConflict {
		t.Fatalf("a save on the older version → %d, want 409", status)
	}

	var retested importRunDTO
	if status := testCall(t, e, "/v1/imports/"+tested.ID+"/validate", AnyMap{
		"version": saved.Version,
	}, &retested); status != http.StatusAccepted || retested.Status != "awaiting_approval" || retested.OnBrokenLink != "drop_link" {
		t.Fatalf("validate → %d %+v, want 202, then awaiting approval with drop_link", status, retested)
	}
	if runs := importRuns(t, e); len(runs) != 1 || runs[0].ID != tested.ID {
		t.Fatalf("runs = %+v, want only %s", runs, tested.ID)
	}

	if status := importCall(t, e, tested.ID, "approve", AnyMap{
		"version": retested.Version - 1,
	}, nil); status != http.StatusConflict {
		t.Fatalf("approving an older version → %d, want 409", status)
	}
	if status := importCall(t, e, tested.ID, "approve", AnyMap{
		"version": retested.Version,
	}, nil); status != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", status)
	}
	if done := readImportRun(t, e, tested.ID); done.Status != "complete" || !done.Released {
		t.Fatalf("the committed run = %+v, want complete with its file released", done)
	}
}

func TestTestsOfOneVersionAtOnceAllAnswerTheOneRun(t *testing.T) {
	e := setupImportApp(t)
	profile, _ := uploadCSV(t, e, "lead", prospectCSV)
	tested, _ := createRunWithMapping(t, e, "lead", profile.SourceRef, profile.SuggestedMapping)
	const clicks = 6
	statuses, bodies := make([]int, clicks), make([]json.RawMessage, clicks)
	var start, done sync.WaitGroup
	start.Add(1)
	for i := range clicks {
		done.Go(func() {
			start.Wait()
			statuses[i] = e.Call(t, http.MethodPost, "/v1/imports/"+tested.ID+"/validate",
				AnyMap{"version": tested.Version}, nil, &bodies[i])
		})
	}
	start.Done()
	done.Wait()

	started := 0
	for i, status := range statuses {
		var answer importRunDTO
		if status == http.StatusAccepted {
			started++
		}
		if !slices.Contains([]int{http.StatusOK, http.StatusAccepted}, status) || json.Unmarshal(bodies[i], &answer) != nil ||
			answer.ID != tested.ID || answer.Status != "validating" {
			t.Errorf("click %d → %d %s, want the one run at validating", i, status, bodies[i])
		}
	}
	if queued := importjobtest.Queued(t, e, compose.ImportValidateArgs{}.Kind()); started != 1 || len(queued) != 1 {
		t.Fatalf("%d clicks started a test and %d test jobs are queued, want one of each", started, len(queued))
	}
	importjobtest.Drain(t, e)
	if run := readImportRun(t, e, tested.ID); run.Status != "awaiting_approval" || run.Version != tested.Version {
		t.Fatalf("after the clicks the run = %+v, want awaiting approval at version %d", run, tested.Version)
	}
	if runs := importRuns(t, e); len(runs) != 1 {
		t.Fatalf("runs = %+v, want only the upload's", runs)
	}
}

func TestCancelReleasesTheSource(t *testing.T) {
	e := setupImportApp(t)
	profile, _ := uploadCSV(t, e, "lead", prospectCSV)

	var cancelled importRunDTO
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+profile.RunID+"/cancel", nil, nil, &cancelled); status != http.StatusOK ||
		cancelled.Status != "cancelled" || !cancelled.Released {
		t.Fatalf("cancel → %d %+v, want 200, cancelled with its file released", status, cancelled)
	}
	var again importRunDTO
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+profile.RunID+"/cancel", nil, nil, &again); status != http.StatusOK ||
		again.Status != "cancelled" {
		t.Fatalf("a second cancel → %d %+v, want 200 with the run as it stands", status, again)
	}
	if status := testCall(t, e, "/v1/imports", AnyMap{
		"connector": "csv", "object": "lead", "source_ref": profile.SourceRef, "mapping": profile.SuggestedMapping,
	}, nil); status != http.StatusConflict {
		t.Fatalf("testing a released upload → %d, want 409: its file is on its way out", status)
	}
}

// testFairRun uploads the prospect file and tests it under one source name.
func testFairRun(t *testing.T, e *apptest.AppEnv) importRunDTO {
	const sourceName = "Fair 2026"
	t.Helper()
	profile, _ := uploadCSV(t, e, "lead", prospectCSV)
	var run importRunDTO
	if status := testCall(t, e, "/v1/imports", AnyMap{
		"connector": "csv", "object": "lead", "source_ref": profile.SourceRef,
		"mapping": profile.SuggestedMapping, "source_name": sourceName,
	}, &run); status != http.StatusAccepted || run.Status != "awaiting_approval" {
		t.Fatalf("testing %q → %d %+v, want 202 awaiting approval", sourceName, status, run)
	}
	return run
}

func TestAnUndoClosesAWaitingReportOfItsName(t *testing.T) {
	e := setupImportApp(t)
	done := testFairRun(t, e)
	if status := importCall(t, e, done.ID, "approve", nil, nil); status != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", status)
	}
	waiting := testFairRun(t, e)

	if status := importCall(t, e, done.ID, "undo", nil, nil); status != http.StatusAccepted {
		t.Fatalf("undo beside a report of the name → %d, want 202", status)
	}
	if got := readImportRun(t, e, waiting.ID); got.Status != "cancelled" || got.SupersededBy != done.ID || !got.Released {
		t.Fatalf("the waiting report = %+v, want cancelled, superseded by the undone run %s and released", got, done.ID)
	}
}

func TestApprovingAReplacedReportNamesTheRunThatReplacedIt(t *testing.T) {
	e := setupImportApp(t)
	older := testFairRun(t, e)
	newer := testFairRun(t, e)

	var refusal struct {
		Details struct {
			RunID string `json:"run_id"`
		} `json:"details"`
	}
	if status := importCall(t, e, older.ID, "approve", nil, &refusal); status != http.StatusConflict ||
		refusal.Details.RunID != newer.ID {
		t.Fatalf("approving the replaced report → %d naming %q, want 409 naming %s", status, refusal.Details.RunID, newer.ID)
	}
}

func TestTheUpdateOpsRefuseAnAgent(t *testing.T) {
	e := setupImportApp(t)
	profile, _ := uploadCSV(t, e, "lead", prospectCSV)
	var minted struct {
		Token string `json:"token"`
	}
	if status := e.Call(t, http.MethodPost, "/v1/passports", AnyMap{
		"label": "import probe", "scopes": []string{"read", "write"},
	}, nil, &minted); status != http.StatusCreated {
		t.Fatalf("issue passport → %d", status)
	}
	bearer := map[string]string{"Authorization": "Bearer " + minted.Token}

	for _, op := range []struct {
		method, path string
		body         AnyMap
	}{
		{http.MethodPatch, "/v1/imports/" + profile.RunID, AnyMap{"version": 1, "draft_step": "map"}},
		{http.MethodPost, "/v1/imports/" + profile.RunID + "/validate", AnyMap{"version": 1}},
		{http.MethodPost, "/v1/imports/" + profile.RunID + "/cancel", nil},
	} {
		if status := e.Call(t, op.method, op.path, op.body, bearer, nil); status != http.StatusForbidden {
			t.Errorf("agent %s %s → %d, want 403: the contract declares it human-only", op.method, op.path, status)
		}
	}
	if draft := readImportRun(t, e, profile.RunID); draft.Status != "pending" || draft.Version != 1 {
		t.Fatalf("the draft after the refusals = %+v, want it untouched", draft)
	}
}
