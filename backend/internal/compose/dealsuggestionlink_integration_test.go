// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestSuggestionLinksExistingDealWithoutCreatingAnother(t *testing.T) {
	e := setupScout(t)
	e.now = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	company := e.SeedCompany(t, "Maple Studio", nil)
	person := e.employee(t, "Mira Buyer", company)
	meeting := e.meeting(e.Admin(), t, "Project scope", &person, e.daysAgo(2))
	e.buyerPair(t, company, e.daysAgo(1))
	e.pass(t)
	suggestion := e.onlySuggestion(e.signalReader(), t, company)
	companyID := ids.From[ids.CompanyKind](company)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Maple rollout", CompanyID: &companyID, PipelineID: e.pipeline, StageID: e.open, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	dealID := ids.UUID(deal.Id)
	result, err := e.decider().AcceptSuggestion(e.signalReader(), suggestion.ID, deals.AcceptSuggestionInput{ExistingDealID: &dealID})
	if err != nil {
		t.Fatal(err)
	}
	if result.DealID != dealID || result.Suggestion.State != deals.SuggestionAccepted {
		t.Fatalf("link result = %+v", result)
	}
	if count := e.WsCount(t, `SELECT count(*) FROM deal WHERE company_id = $1`, company); count != 1 {
		t.Fatalf("deal count = %d, want 1", count)
	}
	if count := e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND deal_id = $2`, meeting, dealID); count != 1 {
		t.Fatalf("linked evidence = %d, want 1", count)
	}
}
