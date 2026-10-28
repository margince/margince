// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/dealstatus"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheRecommendationPreflightRefusesAnObsoleteOperand(t *testing.T) {
	e := setupVerdicts(t)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	cards := newDealStatusService(e.Pool, func() time.Time { return at })
	card, err := cards.ReadFacts(reader, ids.From[ids.DealKind](e.calm))
	if err != nil || card.Next == nil || card.Next.Recommendation == nil {
		t.Fatalf("current recommendation = %+v, %v", card.Next, err)
	}
	receipt, factsOnly := card.Next.Recommendation, true
	params := crmcontracts.GetDealStatusParams{
		FactsOnly: &factsOnly, IfRecommendation: &receipt.BasisFingerprint, IfRule: &receipt.Rule,
	}
	handler := dealstatus.NewHandlers(cards)
	check := func(params crmcontracts.GetDealStatusParams, want int) {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/status", nil).WithContext(reader)
		response := httptest.NewRecorder()
		handler.GetDealStatus(response, request, crmcontracts.Id(e.calm), params)
		if response.Code != want {
			t.Fatalf("recommendation check=%d, want%d: %s", response.Code, want, response.Body.String())
		}
	}
	check(params, http.StatusOK)
	check(crmcontracts.GetDealStatusParams{IfRecommendation: &receipt.BasisFingerprint}, http.StatusBadRequest)
	check(crmcontracts.GetDealStatusParams{IfRecommendation: &receipt.BasisFingerprint, IfRule: &receipt.Rule}, http.StatusBadRequest)
	subject := "An agreed human task"
	if _, _, err := e.Activities.LogActivity(reader, activities.LogActivityInput{
		Kind: "task", Subject: &subject, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: e.calm}},
	}); err != nil {
		t.Fatal(err)
	}
	check(params, http.StatusConflict)
}
