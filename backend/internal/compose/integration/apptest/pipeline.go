// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"net/http"
	"testing"
)

// SeededStages is the seeded default pipeline's stage vocabulary a scenario
// advances deals through.
//
// This and the two fixtures below live here rather than beside the end-to-end
// suite that first needed them because they are keyed on AppEnv: a suite package
// split out of internal/compose/integration can import this package, and nothing
// at all from that package's _test.go files.
type SeededStages struct {
	PipelineID string
	Open       string
	Won        string
	Lost       string
}

// DiscoverSeededPipeline asserts the bootstrap seeded exactly one default
// pipeline with its six stages and resolves the semantic stage ids.
func DiscoverSeededPipeline(t *testing.T, e *AppEnv) SeededStages {
	t.Helper()
	var pipelines struct {
		Data []struct {
			ID        string `json:"id"`
			IsDefault bool   `json:"is_default"`
			Stages    []struct {
				ID       string `json:"id"`
				Semantic string `json:"semantic"`
			} `json:"stages"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/pipelines", nil, nil, &pipelines); status != http.StatusOK {
		t.Fatalf("pipelines status = %d", status)
	}
	if len(pipelines.Data) != 1 || !pipelines.Data[0].IsDefault || len(pipelines.Data[0].Stages) != 6 {
		t.Fatalf("got %+v, want exactly one default pipeline with six stages — the shape the bootstrap seed creates, which every caller of this fixture builds on",
			pipelines.Data)
	}
	stages := SeededStages{PipelineID: pipelines.Data[0].ID}
	for _, s := range pipelines.Data[0].Stages {
		switch s.Semantic {
		case "won":
			stages.Won = s.ID
		case "lost":
			stages.Lost = s.ID
		case "open":
			if stages.Open == "" {
				stages.Open = s.ID
			}
		}
	}
	return stages
}

// ExerciseDealToWon creates the company and deal and closes it as won,
// through the real endpoints, and returns the deal id.
//
// It asserts only what it needs to build that state: a caller wanting a won deal
// should not have to read this to learn which unrelated refusals it also checks.
// The deal_lost_reason refusal it used to carry is a named test of its own in the
// parent suite (TestAdvancingToLostWithoutAReasonIsRefused) — a spec-governed 422
// whose failure must name itself, not surface as a webhooks payload test.
func ExerciseDealToWon(t *testing.T, e *AppEnv, stages SeededStages) string {
	t.Helper()
	dealID := CreateOpenDeal(t, e, stages)

	var deal map[string]any
	// The win gate (ADR-0109 §6): a won deal points at a signed agreement or
	// says why it cannot, and a fixture has no contract in this database by
	// construction — which is precisely what `imported` means.
	status := e.Call(t, "POST", "/v1/deals/"+dealID+"/advance", map[string]any{
		"to_stage_id":                 stages.Won,
		"won_without_contract_reason": "imported",
	}, nil, &deal)
	if status != http.StatusOK || deal["status"] != "won" || deal["closed_at"] == nil {
		t.Fatalf("advance to won = %d %v", status, deal)
	}
	return dealID
}

// CreateOpenDeal creates a company and a deal in the pipeline's open stage,
// through the real endpoints, and returns the deal id. Separate from
// ExerciseDealToWon because a suite asserting what happens to an OPEN deal — the
// refusals around closing it, say — needs the state without the close.
func CreateOpenDeal(t *testing.T, e *AppEnv, stages SeededStages) string {
	t.Helper()
	var company map[string]any
	status := e.Call(t, "POST", "/v1/companies", map[string]any{
		"display_name": "Acme GmbH",
		"source":       "ui",
		"domains":      []map[string]any{{"domain": "acme.example", "is_primary": true}},
	}, nil, &company)
	if status != http.StatusCreated {
		t.Fatalf("create company = %d %v", status, company)
	}

	var deal map[string]any
	status = e.Call(t, "POST", "/v1/deals", map[string]any{
		//nolint:goconst // a request-body key, spelled where the body is written; the
		// other "name" in this package is a key READ off an MCP tool listing, and one
		// constant over both would assert the two wires agree about the word
		"name":         "Acme rollout",
		"amount_minor": 250_000_00,
		"currency":     "EUR",
		"pipeline_id":  stages.PipelineID,
		"stage_id":     stages.Open,
		"company_id":   company["id"],
		"source":       "ui",
	}, nil, &deal)
	if status != http.StatusCreated {
		t.Fatalf("create deal = %d %v", status, deal)
	}
	dealID, ok := deal["id"].(string)
	if !ok {
		t.Fatalf("the created deal carries no string id: %v", deal)
	}
	return dealID
}

// StakeOnOpenDeal creates a bare deal (no company) in the pipeline's
// open stage and stakes personID on it as a deal_stakeholder, through the
// real endpoints, and returns the deal id.
//
// Gives a consent fixture real transactional evidence, since a bare purpose
// claim no longer supplies it on its own — resolveCategory's live-deal arm
// reads exactly this shape: an open deal, this person staked on it.
func StakeOnOpenDeal(t *testing.T, e *AppEnv, name string, stages SeededStages, personID string) string {
	t.Helper()
	var deal map[string]any
	if status := e.Call(t, "POST", "/v1/deals", map[string]any{
		"name": name, "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "source": "manual",
	}, nil, &deal); status != http.StatusCreated {
		t.Fatalf("create deal → %d %v", status, deal)
	}
	dealID, ok := deal["id"].(string)
	if !ok {
		t.Fatalf("the created deal carries no string id: %v", deal)
	}
	if status := e.Call(t, "POST", "/v1/relationships", map[string]any{
		"kind": "deal_stakeholder", "deal_id": dealID, "person_id": personID, "source": "manual",
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("stake the recipient on the deal → %d", status)
	}
	return dealID
}
