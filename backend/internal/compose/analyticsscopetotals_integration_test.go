// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A rep who owns no deal reads the company's totals, learns from the server
// that their own scope is empty, and still cannot assert about the company.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestARepWithNoDealsReadsCompanyTotalsAndIsToldTheirScopeIsEmpty(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	e.seedOpenDeal(t, "Colleague's", 20, &e.Rep3, &amount, nil)
	rep := e.forecastReader(e.ownLensRepCtx(e.Rep1))

	holds := func(scope forecasting.Scope) bool {
		var out bool
		err := forecasting.NewStore(InstallationDB(e.Pool)).InTx(rep, func(ctx context.Context, tx pgx.Tx) error {
			var err error
			out, err = ForecastScopeHoldsDeals(ctx, tx, scope)
			return err
		})
		if err != nil {
			t.Fatalf("asking %q: %v", scope.Kind, err)
		}
		return out
	}
	if !holds(forecasting.Scope{Kind: forecasting.ScopeWorkspace}) {
		t.Error("the company holds a deal, but the rep was told it does not")
	}
	if holds(forecasting.Scope{}) {
		t.Error("the rep owns no deal, but their own scope was reported as holding one")
	}

	if _, err := e.writableScope(rep, forecasting.Scope{Kind: forecasting.ScopeWorkspace}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("recording against the company got %v, want permission denied", err)
	}
	foreign := ids.NewV7()
	if _, err := e.writableScope(rep, forecasting.Scope{Kind: forecasting.ScopeTeam, ID: &foreign}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("recording against a foreign team got %v, want not found", err)
	}
}

func TestARepIsOfferedTheCompanyAndEveryLiveTeam(t *testing.T) {
	e := setupForecast(t)
	rep := e.ownLensRepCtx(e.Rep1)
	err := forecasting.NewStore(InstallationDB(e.Pool)).InTx(e.forecastReader(rep), func(ctx context.Context, tx pgx.Tx) error {
		got, err := analyticsContextFor(ctx, tx, clocktest.Now(t))
		if err != nil {
			return err
		}
		kinds := map[string]bool{}
		for _, s := range got.AllowedScopes {
			kinds[string(s.Kind)] = true
		}
		if !kinds["workspace"] || !kinds["team"] || !kinds["owner"] {
			t.Errorf("a rep is offered %+v, want the company, the live teams and themselves", got.AllowedScopes)
		}
		if got.DefaultScope.Kind != "owner" || got.Capabilities.ViewManagerForecast {
			t.Errorf("the rep's default moved: %+v", got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
