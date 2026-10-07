// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// The deals list answers a search the way the contacts and leads lists do:
// a word of the name finds the deal, and a deal whose name does not hold it
// stays off the page.
func TestTheDealsListFindsADealByAWordOfItsName(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	pipelineID, stageID, _ := companyRollupOpenStage(t, e)
	retrofit := createSearchableDeal(t, e, pipelineID, stageID, "Fleet retrofit")
	rollout := createSearchableDeal(t, e, pipelineID, stageID, "Depot rollout")

	found := listDealIDs(t, e, "retrofit")
	if !found[retrofit] || found[rollout] {
		t.Fatalf("q=retrofit returned %v, want only the retrofit deal %s", found, retrofit)
	}
	// A fragment inside a word is the substring arm, not the full-text one.
	if found := listDealIDs(t, e, "ollou"); !found[rollout] || found[retrofit] {
		t.Fatalf("q=ollou returned %v, want only the rollout deal %s", found, rollout)
	}
}

func createSearchableDeal(t *testing.T, e *apptest.AppEnv, pipelineID, stageID, name string) string {
	t.Helper()
	var deal struct {
		ID string `json:"id"`
	}
	status := e.Call(t, "POST", "/v1/deals", AnyMap{
		"name": name, "amount_minor": 100_000, "currency": "EUR",
		"pipeline_id": pipelineID, "stage_id": stageID, "source": "manual",
	}, nil, &deal)
	if status != http.StatusCreated {
		t.Fatalf("create deal %q = %d", name, status)
	}
	return deal.ID
}

func listDealIDs(t *testing.T, e *apptest.AppEnv, q string) map[string]bool {
	t.Helper()
	var page struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/deals?q="+url.QueryEscape(q), nil, nil, &page); status != http.StatusOK {
		t.Fatalf("GET /v1/deals?q=%s = %d", q, status)
	}
	ids := make(map[string]bool, len(page.Data))
	for _, d := range page.Data {
		ids[d.ID] = true
	}
	return ids
}
