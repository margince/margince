// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAThinProbabilityGroupWithholdsTheCompanyForecast(t *testing.T) {
	f := newStageCohortFixture(t)
	typed := f.stage(0)
	others := f.env.addMembers(t, f.env.Team1, "two@forecast.test", "three@forecast.test", "four@forecast.test", "five@forecast.test")
	owners := append([]ids.UUID{f.env.Rep1}, others...)
	today := clocktest.Now(t)
	ownDeal := f.openDealOf(t, owners[0], typed, today)
	singledOut := f.openDealOf(t, owners[1], typed, today)
	for _, owner := range owners[2:] {
		f.openDealOf(t, owner, typed, today)
	}
	reader, ok := principal.Actor(f.forecastReader())
	if !ok {
		t.Fatal("forecast reader has no principal")
	}
	reader.Permissions.RoleKeys = []string{"admin"}
	reader.Permissions.Objects["deal"] = principal.ObjectGrant{Read: true, Update: true}
	ctx := principal.WithCorrelationID(principal.WithActor(f.forecastReader(), reader), ids.NewV7())
	handlers := newForecastHandlers(f.env.Pool, func() time.Time { return today })
	ask := func(scope string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/forecast?as_of="+today.Format(time.DateOnly)+"&contributions=true&"+scope, nil).WithContext(ctx)
		crmcontracts.Handler(Server{forecastHandlers: handlers}).ServeHTTP(rec, req)
		return rec
	}
	f.env.showIndividualPerformance(t, false)
	control := ask("scope_kind=workspace")
	var before crmcontracts.ForecastReadings
	decodeWire(t, control, http.StatusOK, &before)
	if before.OpenMinor != int64(len(owners))*1_000_000 || before.Contributions == nil || len(*before.Contributions) != 1 {
		t.Fatal("the unsplit company total or own contribution was missing")
	}
	f.env.showIndividualPerformance(t, true)
	chance := 77
	for _, deal := range []ids.UUID{ownDeal, singledOut} {
		if _, err := f.store.UpdateDeal(ctx, ids.From[ids.DealKind](deal), deals.UpdateDealInput{WinProbability: &chance}); err != nil {
			t.Fatal(err)
		}
	}
	shown := f.forecastOverHTTP(t, today)
	row := contributionOfDeal(t, *shown.Contributions, singledOut)
	if row.ProbabilitySource == nil || string(*row.ProbabilitySource) != "human" {
		t.Fatal("the singleton deal did not use the human probability")
	}
	f.env.showIndividualPerformance(t, false)
	for _, scope := range []string{"", "scope_kind=workspace"} {
		rec := ask(scope)
		var problem struct {
			Code string `json:"code"`
		}
		body := rec.Body.String()
		decodeWire(t, rec, http.StatusForbidden, &problem)
		if problem.Code != individualOffCode || !strings.Contains(body, "forecast is withheld in full") {
			t.Fatalf("company forecast refusal = %s", rec.Body.String())
		}
	}
	own := ask("scope_kind=owner&scope_id=" + f.env.Rep1.String())
	var personal crmcontracts.ForecastReadings
	decodeWire(t, own, http.StatusOK, &personal)
	if personal.Contributions == nil || len(*personal.Contributions) != 1 || personal.WeightedMinor <= 0 {
		t.Fatal("the owner did not receive their actual figures")
	}
	if got := contributionOfDeal(t, *personal.Contributions, ownDeal); got.ProbabilitySource == nil || string(*got.ProbabilitySource) != "human" {
		t.Fatal("the personal forecast omitted the owner's edited deal")
	}
}
