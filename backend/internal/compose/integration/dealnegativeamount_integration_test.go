// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A deal's price is zero or more through every door that sets it: a create, an
// edit and the acceptance of a Deal Scout suggestion. The three share one rule
// (moneyPairError), so this suite drives each over HTTP and expects the same
// refusal, naming amount_minor.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func namesAmountNegative(problem problemBody) bool {
	for _, refusal := range problem.Details.Errors {
		if refusal.Field == "amount_minor" && refusal.Code == "amount_negative" {
			return true
		}
	}
	return false
}

func TestNoDoorLetsADealBeNegative(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	stages := apptest.DiscoverSeededPipeline(t, e)

	t.Run("create", func(t *testing.T) {
		var problem problemBody
		status := e.Call(t, "POST", "/v1/deals", AnyMap{
			"name": "Below zero", "pipeline_id": stages.PipelineID, "stage_id": stages.Open,
			"source": "manual", "amount_minor": -5, "currency": "EUR",
		}, nil, &problem)
		if status != http.StatusUnprocessableEntity || !namesAmountNegative(problem) {
			t.Fatalf("create → %d %+v, want 422 amount_negative on amount_minor", status, problem)
		}
	})

	t.Run("edit", func(t *testing.T) {
		deal := createAndID(t, e, "/v1/deals", AnyMap{
			"name": "Priced", "pipeline_id": stages.PipelineID, "stage_id": stages.Open,
			"source": "manual", "amount_minor": 1000, "currency": "EUR",
		})
		var problem problemBody
		status := e.Call(t, "PATCH", "/v1/deals/"+deal, AnyMap{"amount_minor": -1}, map[string]string{"If-Match": "1"}, &problem)
		if status != http.StatusUnprocessableEntity || !namesAmountNegative(problem) {
			t.Fatalf("edit → %d %+v, want 422 amount_negative on amount_minor", status, problem)
		}
		var read struct {
			AmountMinor int64 `json:"amount_minor"`
		}
		if status := e.Call(t, "GET", "/v1/deals/"+deal, nil, nil, &read); status != http.StatusOK || read.AmountMinor != 1000 {
			t.Fatalf("the refused edit left %d (status %d), want 1000", read.AmountMinor, status)
		}
	})

	t.Run("accepting a suggestion", func(t *testing.T) {
		_, suggestionID := seedScoutedCompany(t, e)
		path := "/v1/deal-suggestions/" + suggestionID + "/accept"
		var problem problemBody
		status := e.Call(t, "POST", path, AnyMap{"amount_minor": -500, "currency": "EUR"},
			map[string]string{"Idempotency-Key": "neg-accept"}, &problem)
		if status != http.StatusUnprocessableEntity || !namesAmountNegative(problem) {
			t.Fatalf("accept → %d %+v, want 422 amount_negative on amount_minor", status, problem)
		}
		var list suggestionPage
		if status := e.Call(t, "GET", "/v1/deal-suggestions", nil, nil, &list); status != http.StatusOK || len(list.Data) == 0 {
			t.Fatalf("the refused acceptance must leave the suggestion open: %d %+v", status, list)
		}
	})
}

// The sign rule judges the amount a write SETS. A deal that already holds a
// negative amount stays editable on every other field.
func TestADealAlreadyBelowZeroStaysEditableOnOtherFields(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	id := e.SeedDeal(t, "Older deal", pipeline, open, nil)
	e.WsExec(t, `UPDATE deal SET amount_minor = -5, currency = 'EUR' WHERE id = $1`, id)

	renamed := "Renamed"
	dealID := ids.From[ids.DealKind](id)
	got, err := e.Deals.UpdateDeal(e.Admin(), dealID, deals.UpdateDealInput{Name: &renamed})
	if err != nil {
		t.Fatalf("renaming a deal that holds a negative amount: %v", err)
	}
	if got.Name != renamed || got.AmountMinor == nil || *got.AmountMinor != -5 {
		t.Fatalf("after the rename = %q amount %v, want the new name and the amount untouched", got.Name, got.AmountMinor)
	}

	same := int64(-5)
	if _, err := e.Deals.UpdateDeal(e.Admin(), dealID, deals.UpdateDealInput{AmountMinor: &same}); err != nil {
		t.Fatalf("re-sending the amount the deal already holds sets nothing: %v", err)
	}
	lower := int64(-6)
	if _, err := e.Deals.UpdateDeal(e.Admin(), dealID, deals.UpdateDealInput{AmountMinor: &lower}); err == nil {
		t.Fatal("moving a deal's amount to another negative figure must be refused")
	}
}
