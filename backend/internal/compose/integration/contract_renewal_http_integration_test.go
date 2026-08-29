// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A renewal names its own deal ON THE WIRE.
//
// The store-level case beside this one proves the successor keeps a deal it is
// handed; this proves the request can hand it one at all. They are different
// claims and only this one fails when the request field stops being mapped —
// which is the state the API was in: `RenewContractRequest` had no `deal_id`,
// so every renewal successor was created attached to nothing, and its PDF was
// out of reach of the deal room the renewal is discussed in.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestARenewalOverHTTPCarriesTheDealTheRequestNames(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var org struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/organizations",
		map[string]any{"display_name": "Acme"}, nil, &org); status != http.StatusCreated {
		t.Fatalf("creating the counterparty → %d, want 201", status)
	}

	// The pipeline the installation ships with, so the deal has a stage to sit
	// in without this case inventing a second answer to what a pipeline is.
	var pipelines struct {
		Data []struct {
			ID     string `json:"id"`
			Stages []struct {
				ID       string `json:"id"`
				Semantic string `json:"semantic"`
			} `json:"stages"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/pipelines", nil, nil, &pipelines); status != http.StatusOK {
		t.Fatalf("reading the pipelines → %d, want 200", status)
	}
	if len(pipelines.Data) == 0 || len(pipelines.Data[0].Stages) == 0 {
		t.Fatal("the installation ships no pipeline with stages, so this case has nowhere to put a deal")
	}
	var openStage string
	for _, stage := range pipelines.Data[0].Stages {
		if stage.Semantic == "open" {
			openStage = stage.ID
			break
		}
	}
	if openStage == "" {
		t.Fatal("the default pipeline has no open stage")
	}

	var renewalDeal struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/deals", map[string]any{
		"name": "Acme renewal 2027", "pipeline_id": pipelines.Data[0].ID,
		"stage_id": openStage, "organization_id": org.ID,
	}, nil, &renewalDeal); status != http.StatusCreated {
		t.Fatalf("creating the renewal deal → %d, want 201", status)
	}

	var first struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/contracts", map[string]any{
		"organization_id": org.ID, "title": "MSA 2026", "value_basis": "total",
	}, nil, &first); status != http.StatusCreated {
		t.Fatalf("creating the predecessor → %d, want 201", status)
	}
	if status := e.Call(t, "POST", "/v1/contracts/"+first.ID+"/status",
		map[string]any{"status": "active"}, nil, nil); status != http.StatusOK {
		t.Fatalf("activating the predecessor → %d, want 200", status)
	}

	var successor struct {
		ID     string  `json:"id"`
		DealID *string `json:"deal_id"`
	}
	if status := e.Call(t, "POST", "/v1/contracts/"+first.ID+"/renewal", map[string]any{
		"title": "MSA 2027", "value_basis": "annualized_12m", "deal_id": renewalDeal.ID,
	}, nil, &successor); status != http.StatusCreated {
		t.Fatalf("renewing → %d, want 201", status)
	}
	if successor.DealID == nil {
		t.Fatal("the successor came back with no deal_id, so the request field reached nothing — " +
			"a renewal that declares its opportunity is still created attached to none")
	}
	if *successor.DealID != renewalDeal.ID {
		t.Errorf("successor deal_id = %q, want the deal the request named (%q)", *successor.DealID, renewalDeal.ID)
	}
}
