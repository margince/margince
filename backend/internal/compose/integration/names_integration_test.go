// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A name is refused with a 4xx when it is taken, blank or invisible, on every
// write path that holds one: the answer must never be a 500.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestPipelineAndStageNamesAreRefusedWhenTakenOrBlank(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Names E2E", "names@fable.test", "Admin")

	var first, second struct {
		ID string `json:"id"`
	}
	mustCall(t, e, "POST", "/v1/pipelines", AnyMap{"name": "Alpha", "stages": []AnyMap{{"name": "Scout", "position": 1}}}, http.StatusCreated, &first)
	mustCall(t, e, "POST", "/v1/pipelines", AnyMap{"name": "Beta", "stages": []AnyMap{{"name": "Scout", "position": 1}}}, http.StatusCreated, &second)

	mustCall(t, e, "PATCH", "/v1/pipelines/"+second.ID, AnyMap{"name": "Alpha"}, http.StatusConflict, nil)
	for _, blank := range []string{"", "   ", "\u200b", " \u200b\u00a0"} {
		mustCall(t, e, "POST", "/v1/pipelines", AnyMap{"name": blank}, http.StatusUnprocessableEntity, nil)
		mustCall(t, e, "PATCH", "/v1/pipelines/"+first.ID, AnyMap{"name": blank}, http.StatusUnprocessableEntity, nil)
	}

	var stage struct {
		ID string `json:"id"`
	}
	mustCall(t, e, "POST", "/v1/stages", AnyMap{"pipeline_id": first.ID, "name": "Qualified", "position": 2}, http.StatusCreated, &stage)
	for _, blank := range []string{"", "   ", "\u200b"} {
		mustCall(t, e, "PATCH", "/v1/stages/"+stage.ID, AnyMap{"name": blank}, http.StatusUnprocessableEntity, nil)
		mustCall(t, e, "POST", "/v1/stages", AnyMap{"pipeline_id": first.ID, "name": blank, "position": 3}, http.StatusUnprocessableEntity, nil)
	}
	var kept struct {
		Name string `json:"name"`
	}
	mustCall(t, e, "GET", "/v1/stages/"+stage.ID, nil, http.StatusOK, &kept)
	if kept.Name != "Qualified" {
		t.Fatalf("a refused rename changed the stage to %q", kept.Name)
	}
}

func TestTeamNamesAreRefusedWhenTakenOrInvisible(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Teams E2E", "teams@fable.test", "Admin")

	var old struct {
		ID string `json:"id"`
	}
	mustCall(t, e, "POST", "/v1/teams", AnyMap{"name": "Support"}, http.StatusCreated, &old)
	mustCall(t, e, "PATCH", "/v1/teams/"+old.ID, AnyMap{"archived": true}, http.StatusOK, nil)
	mustCall(t, e, "POST", "/v1/teams", AnyMap{"name": "Support"}, http.StatusCreated, nil)

	mustCall(t, e, "PATCH", "/v1/teams/"+old.ID, AnyMap{"archived": false}, http.StatusConflict, nil)

	for _, invisible := range []string{"\u200b", "\u200b\u200b", " \u2060 "} {
		mustCall(t, e, "POST", "/v1/teams", AnyMap{"name": invisible}, http.StatusUnprocessableEntity, nil)
	}
}
