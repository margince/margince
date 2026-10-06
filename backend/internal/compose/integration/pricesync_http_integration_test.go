// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// Auto-sync is on until somebody turns it off, and the switch is audited.
func TestThePriceSyncSwitchDefaultsOnAndIsAudited(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	var state struct {
		AutoSync bool      `json:"auto_sync"`
		LastRun  *struct{} `json:"last_run"`
	}
	if status := e.Call(t, "GET", "/v1/ai/price-sync", nil, nil, &state); status != http.StatusOK || !state.AutoSync || state.LastRun != nil {
		t.Fatalf("GET → %d %+v, want 200, auto_sync on, never run", status, state)
	}
	if status := e.Call(t, "PUT", "/v1/ai/price-sync", map[string]bool{"auto_sync": false}, nil, &state); status != http.StatusOK || state.AutoSync {
		t.Fatalf("PUT off → %d %+v", status, state)
	}
	if status := e.Call(t, "PUT", "/v1/ai/price-sync", map[string]string{}, nil, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("PUT without auto_sync → %d, want 422", status)
	}
	var n int
	if err := e.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE entity_type = 'ai_model_rate' AND after ? 'ai.price_sync'`).Scan(&n); err != nil {
		t.Fatalf("counting the switch's audit rows: %v", err)
	}
	if n != 1 {
		t.Errorf("%d audit rows for the switch, want 1", n)
	}
}

// Refresh now is a run like the daily one: the card reads what it did.
func TestRefreshNowIsRecordedAsTheLastRun(t *testing.T) {
	e := apptest.SetupAppWithOptions(t, compose.WithPriceCatalogues(
		staticCatalogue(`{"data":[{"id":"a/listed","pricing":{"prompt":"0.000001","completion":"0.000002"}}]}`),
		staticCatalogue(`{"google":{"id":"google","models":{}}}`)))
	e.BootstrapWorkspace(t)
	if status := e.Call(t, "POST", "/v1/ai-model-rates/refresh", nil, nil, nil); status != http.StatusOK {
		t.Fatalf("refresh → %d, want 200", status)
	}
	var state struct {
		LastRun *struct {
			Trigger string `json:"trigger"`
			Report  struct {
				Providers []struct {
					Provider string `json:"provider"`
				} `json:"providers"`
			} `json:"report"`
		} `json:"last_run"`
	}
	if status := e.Call(t, "GET", "/v1/ai/price-sync", nil, nil, &state); status != http.StatusOK || state.LastRun == nil {
		t.Fatalf("GET → %d %+v, want the run recorded", status, state)
	}
	if state.LastRun.Trigger != "manual" || len(state.LastRun.Report.Providers) == 0 {
		t.Errorf("last run = %+v, want a manual run with a line per provider", state.LastRun)
	}
}

// staticCatalogue answers a public price list from memory, so no test reaches the network.
type staticCatalogue string

func (c staticCatalogue) Fetch(context.Context) ([]byte, error) { return []byte(c), nil }
