// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The Deal Scout routes' other answers over HTTP: paging and filtering the
// list, the refusals an acceptance and a dismissal give, and the corrections
// an acceptance carries onto the deal it opens.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type suggestionPage struct {
	Data []AnyMap `json:"data"`
	Page struct {
		HasMore    bool    `json:"has_more"`
		NextCursor *string `json:"next_cursor"`
	} `json:"page"`
}

func TestTheSuggestionListPagesAndFilters(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	stages := apptest.DiscoverSeededPipeline(t, e)
	seedScoutedNamed(t, e, "Acme GmbH", "Dana Buyer")
	seedScoutedNamed(t, e, "Globex AG", "Lee Buyer")

	var first, second suggestionPage
	if status := e.Call(t, "GET", "/v1/deal-suggestions?limit=1&pipeline_id="+stages.PipelineID, nil, nil, &first); status != http.StatusOK {
		t.Fatalf("first page = %d", status)
	}
	if len(first.Data) != 1 || !first.Page.HasMore || first.Page.NextCursor == nil {
		t.Fatalf("first page = %+v, want one row and a cursor to the next", first)
	}
	if status := e.Call(t, "GET", "/v1/deal-suggestions?limit=1&cursor="+*first.Page.NextCursor, nil, nil, &second); status != http.StatusOK {
		t.Fatalf("second page = %d", status)
	}
	if len(second.Data) != 1 || second.Page.HasMore || second.Data[0]["id"] == first.Data[0]["id"] {
		t.Fatalf("second page = %+v, want the other suggestion and no more", second)
	}
	var otherStage suggestionPage
	if status := e.Call(t, "GET", "/v1/deal-suggestions?stage_id="+stages.Won, nil, nil, &otherStage); status != http.StatusOK || len(otherStage.Data) != 0 {
		t.Fatalf("filtering on a stage no suggestion opens in = %d with %d rows, want none", status, len(otherStage.Data))
	}
	var whole suggestionPage
	if status := e.Call(t, "GET", "/v1/deal-suggestions?limit=1000", nil, nil, &whole); status != http.StatusOK || len(whole.Data) != 2 {
		t.Fatalf("a limit past the ceiling = %d with %d rows, want both, clamped rather than refused", status, len(whole.Data))
	}
	if status := e.Call(t, "GET", "/v1/deal-suggestions?cursor=not-a-cursor", nil, nil, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("a malformed cursor = %d, want 422", status)
	}
}

func TestAnAcceptanceRefusesWhatItCannotDo(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	stages := apptest.DiscoverSeededPipeline(t, e)
	_, suggestionID := seedScoutedCompany(t, e)
	path := "/v1/deal-suggestions/" + suggestionID + "/accept"
	key := func(k string) map[string]string { return map[string]string{"Idempotency-Key": k} }

	if status := e.Call(t, "POST", path, "not an object", key("a0"), nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("a body that is not an object = %d, want 422", status)
	}
	var conflicting AnyMap
	if status := e.Call(t, "POST", path, AnyMap{"no_amount": true, "amount_minor": 100, "currency": "EUR"}, key("a1"), &conflicting); status != http.StatusUnprocessableEntity {
		t.Fatalf("no_amount with an amount = %d %v, want 422", status, conflicting)
	}
	if status := e.Call(t, "POST", path, AnyMap{"stage_id": stages.Won}, key("a2"), nil); status != http.StatusNotFound {
		t.Fatalf("opening in a closed stage = %d, want 404", status)
	}
	if status := e.Call(t, "POST", path, AnyMap{"owner_id": "01a0e9f1-0000-7000-8000-00000000dead"}, key("a3"), nil); status < 400 {
		t.Fatalf("an owner who is not a seat = %d, want a refusal", status)
	}
	unknown := "/v1/deal-suggestions/01a0e9f1-0000-7000-8000-00000000beef"
	if status := e.Call(t, "POST", unknown+"/accept", nil, key("a4"), nil); status != http.StatusNotFound {
		t.Fatalf("accepting a suggestion that does not exist = %d, want 404", status)
	}
	if status := e.Call(t, "POST", unknown+"/dismiss", nil, key("a5"), nil); status != http.StatusNotFound {
		t.Fatalf("dismissing a suggestion that does not exist = %d, want 404", status)
	}

	// The refusals changed nothing: the suggestion still opens its deal.
	var accepted AnyMap
	if status := e.Call(t, "POST", path, AnyMap{"close_date": "2099-12-01", "name": "Acme rollout"}, key("a6"), &accepted); status != http.StatusOK {
		t.Fatalf("accepting after the refusals = %d %v", status, accepted)
	}
	var deal AnyMap
	dealID, _ := accepted["deal_id"].(string)
	if status := e.Call(t, "GET", "/v1/deals/"+dealID, nil, nil, &deal); status != http.StatusOK {
		t.Fatalf("reading the deal = %d", status)
	}
	if deal["name"] != "Acme rollout" || deal["expected_close_date"] != "2099-12-01" {
		t.Fatalf("deal = %v, want the reader's name and date", deal)
	}
}

func TestAnAcceptanceAfterADealWasOpenedByHandAnswersSuperseded(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	stages := apptest.DiscoverSeededPipeline(t, e)
	companyID, suggestionID := seedScoutedCompany(t, e)
	if status := e.Call(t, "POST", "/v1/deals", AnyMap{
		"name": "Opened by hand", "pipeline_id": stages.PipelineID, "stage_id": stages.Open,
		"company_id": companyID, "source": "manual",
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("opening the deal by hand = %d", status)
	}

	var refused AnyMap
	if status := e.Call(t, "POST", "/v1/deal-suggestions/"+suggestionID+"/accept", nil,
		map[string]string{"Idempotency-Key": "late"}, &refused); status != http.StatusConflict || refused["code"] != "suggestion_superseded" {
		t.Fatalf("accepting = %d %v, want 409 suggestion_superseded", status, refused)
	}
	var left suggestionPage
	if status := e.Call(t, "GET", "/v1/deal-suggestions?company_id="+companyID, nil, nil, &left); status != http.StatusOK || len(left.Data) != 0 {
		t.Fatalf("the company still lists %d suggestions after one was superseded", len(left.Data))
	}
}
