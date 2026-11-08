// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The company quarter carries a 760k call, and each team carries its own. Every
// seat reads the totals. The history answers only a seat that may assert about
// the population, and refuses the rest as recording a call would.
func TestTheCallHistoryAnswersOnlyASeatThatMayAssertAboutThePopulation(t *testing.T) {
	q := seedTargetQuarter(t)
	recordTeamCall(t, q, q.env.Team1, 30_000_000)
	recordTeamCall(t, q, q.env.Team2, 20_000_000)
	teamLens := reportingTieredReader(q.human, principal.RowScopeTeam)
	ownLens := reportingTieredReader(q.human, principal.RowScopeOwn)

	status, body := callHistoryOverHTTP(q.human, t, q, "scope_kind=workspace")
	if calls := callsIn(t, status, body); len(calls) != 1 || calls[0].AmountMinor != 76_000_000 || calls[0].Note == nil || *calls[0].Note != "Quarter review" {
		t.Fatalf("the all-scope seat's company history %+v, want the 760k call with its note", calls)
	}
	status, body = callHistoryOverHTTP(teamLens, t, q, "scope_kind=team&scope_id="+q.env.Team1.String())
	if calls := callsIn(t, status, body); len(calls) != 1 || calls[0].AmountMinor != 30_000_000 {
		t.Fatalf("the team-lens seat's own team history %+v, want its 300k call", calls)
	}

	refusals := []struct {
		name   string
		seat   context.Context
		query  string
		status int
		code   string
	}{
		{"a team-lens seat asking for the company", teamLens, "scope_kind=workspace", http.StatusForbidden, "permission_denied"},
		{"an own-lens seat asking for the company", ownLens, "scope_kind=workspace", http.StatusForbidden, "permission_denied"},
		{"a team-lens seat naming a team not its own", teamLens, "scope_kind=team&scope_id=" + q.env.Team2.String(), http.StatusNotFound, "not_found"},
		{"an own-lens seat naming its own team", ownLens, "scope_kind=team&scope_id=" + q.env.Team1.String(), http.StatusNotFound, "not_found"},
	}
	for _, refusal := range refusals {
		status, body := callHistoryOverHTTP(refusal.seat, t, q, refusal.query)
		if code := problemCodeOf(t, body); status != refusal.status || code != refusal.code {
			t.Errorf("%s: %d %q, want %d %q: %s", refusal.name, status, code, refusal.status, refusal.code, body)
		}
	}
}

func recordTeamCall(t *testing.T, q targetQuarter, team ids.UUID, minor int64) {
	t.Helper()
	calls := forecasting.NewStore(InstallationDB(q.env.Pool))
	if err := calls.InTx(q.human, func(ctx context.Context, tx pgx.Tx) error {
		period, base, err := ForecastPeriodAt(ctx, tx, forecasting.PeriodQuarter, q.at)
		if err != nil {
			return err
		}
		_, err = calls.RecordCallTx(ctx, tx, forecasting.NewCall{Period: period, Scope: forecasting.Scope{Kind: forecasting.ScopeTeam, ID: &team}, AmountMinor: minor, Currency: base})
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func callHistoryOverHTTP(seat context.Context, t *testing.T, q targetQuarter, scope string) (int, []byte) {
	t.Helper()
	handlers := newForecastHandlers(q.env.Pool, func() time.Time { return q.at })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/forecast/calls?period=quarter&as_of="+q.at.Format(time.DateOnly)+"&"+scope, nil).WithContext(seat)
	crmcontracts.Handler(Server{forecastHandlers: handlers}).ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

func callsIn(t *testing.T, status int, body []byte) []crmcontracts.ForecastCall {
	t.Helper()
	if status != http.StatusOK {
		t.Fatalf("GET /forecast/calls answered %d: %s", status, body)
	}
	var out struct {
		Calls []crmcontracts.ForecastCall `json:"calls"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	return out.Calls
}

func problemCodeOf(t *testing.T, body []byte) string {
	t.Helper()
	var problem struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(body, &problem); err != nil {
		t.Fatalf("the refusal body %s is not a problem: %v", body, err)
	}
	return problem.Code
}
