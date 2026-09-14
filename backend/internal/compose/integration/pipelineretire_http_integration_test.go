// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Retire and restore over the real wire.
//
// The store's own cases live beside them in pipelineretire_integration_test.go
// and say what the operation MEANS: the default cannot be retired while it is
// the default, and retiring moves no deals. What only the wire can say is what a
// caller gets — the statuses, the version guard, and the body the undo answers
// with — and none of that is reachable from the store, which returns a value and
// never a response.
//
// The version guard is the reason this is not only a coverage exercise. Retiring
// is a DELETE with an optional `If-Match`, and a guard nothing exercises is a
// guard nobody knows is wired: a caller holding a stale read must be refused
// rather than retiring a pipeline somebody else already changed.

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// seedPipeline creates one pipeline and returns its id and version.
func seedPipeline(t *testing.T, e *apptest.AppEnv, name string) (id string, version int64) {
	t.Helper()
	var created struct {
		ID      string `json:"id"`
		Version int64  `json:"version"`
	}
	if status := e.Call(t, "POST", "/v1/pipelines", AnyMap{"name": name}, nil, &created); status != http.StatusCreated {
		t.Fatalf("create pipeline %q → %d", name, status)
	}
	return created.ID, created.Version
}

func TestRetireAndRestoreAPipelineOverTheWire(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Pipelines E2E", "pipes@fable.test", "Admin")

	// Two, because the one being retired must not be the default: that refusal
	// is the store's own case, and reaching it here would test it twice and the
	// wire not at all.
	keeper, _ := seedPipeline(t, e, "Standing process")
	var promoted struct{}
	if status := e.Call(t, "PATCH", "/v1/pipelines/"+keeper, AnyMap{"is_default": true}, nil, &promoted); status != http.StatusOK {
		t.Fatalf("making the keeper default → %d", status)
	}
	retiring, version := seedPipeline(t, e, "Old process")

	// A stale If-Match refuses, and the pipeline is still there afterwards.
	var problem struct {
		Code string `json:"code"`
	}
	if status := e.Call(t, "DELETE", "/v1/pipelines/"+retiring, nil,
		map[string]string{"If-Match": "999"}, &problem); status != http.StatusConflict || problem.Code != "version_skew" {
		t.Fatalf("retiring on a stale If-Match → %d %q, want 409 version_skew", status, problem.Code)
	}

	// The caller's own version is accepted.
	if status := e.Call(t, "DELETE", "/v1/pipelines/"+retiring, nil,
		map[string]string{"If-Match": strconv.FormatInt(version, 10)}, nil); status != http.StatusNoContent {
		t.Fatalf("retiring on the caller's own version → %d, want 204", status)
	}

	// Retired means gone from the list a picker reads, and still readable by id
	// — the retirement is a soft flag, not a delete.
	var listed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/pipelines", nil, nil, &listed); status != http.StatusOK {
		t.Fatalf("listing pipelines → %d", status)
	}
	for _, p := range listed.Data {
		if p.ID == retiring {
			t.Error("a retired pipeline is still in the default list, so it is still offered where a new deal is filed")
		}
	}

	// The undo answers the pipeline itself, which is the point of it answering
	// a body at all: a caller sees the state it is in without a second read.
	var restored struct {
		ID         string  `json:"id"`
		ArchivedAt *string `json:"archived_at"`
		IsDefault  bool    `json:"is_default"`
	}
	if status := e.Call(t, "POST", "/v1/pipelines/"+retiring+"/restore", nil, nil, &restored); status != http.StatusOK {
		t.Fatalf("restoring → %d, want 200", status)
	}
	if restored.ID != retiring || restored.ArchivedAt != nil {
		t.Errorf("the restore answered %+v — a caller reading this body must see a pipeline back in use", restored)
	}
	// Putting a pipeline back is not a claim about where new deals go.
	if restored.IsDefault {
		t.Error("the restore made the pipeline default again, which is updatePipeline's to say and not this call's")
	}

	// Restoring one that is not archived changes nothing and answers the same
	// way, so a retried call after a lost response is safe.
	if status := e.Call(t, "POST", "/v1/pipelines/"+retiring+"/restore", nil, nil, &restored); status != http.StatusOK {
		t.Fatalf("restoring an unarchived pipeline → %d, want the same 200 the first call gave", status)
	}
}
