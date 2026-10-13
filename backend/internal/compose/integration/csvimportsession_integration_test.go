// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An upload opens one run, and every later choice, test and approval is made
// on it. A second test and "Import anyway" never open another.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
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
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+tested.ID+"/validate", AnyMap{
		"version": saved.Version,
	}, nil, &retested); status != http.StatusOK || retested.Status != "awaiting_approval" || retested.OnBrokenLink != "drop_link" {
		t.Fatalf("validate → %d %+v, want 200 awaiting approval with drop_link", status, retested)
	}
	if runs := importRuns(t, e); len(runs) != 1 || runs[0].ID != tested.ID {
		t.Fatalf("runs = %+v, want only %s", runs, tested.ID)
	}

	if status := e.Call(t, http.MethodPost, "/v1/imports/"+tested.ID+"/approve", AnyMap{
		"version": retested.Version - 1,
	}, nil, nil); status != http.StatusConflict {
		t.Fatalf("approving an older version → %d, want 409", status)
	}
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+tested.ID+"/approve", AnyMap{
		"version": retested.Version,
	}, nil, nil); status != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", status)
	}
	if done := readImportRun(t, e, tested.ID); done.Status != "complete" || !done.Released {
		t.Fatalf("the committed run = %+v, want complete with its file released", done)
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
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+profile.RunID+"/cancel", nil, nil, nil); status != http.StatusConflict {
		t.Fatalf("a second cancel → %d, want 409", status)
	}
	if status := e.Call(t, http.MethodPost, "/v1/imports", AnyMap{
		"connector": "csv", "object": "lead", "source_ref": profile.SourceRef, "mapping": profile.SuggestedMapping,
	}, nil, nil); status != http.StatusConflict {
		t.Fatalf("testing a released upload → %d, want 409: its file is on its way out", status)
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
