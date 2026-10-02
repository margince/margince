// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/database"
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
	if err := database.WithWorkspaceTx(context.Background(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM audit_log WHERE entity_type = 'ai_model_rate' AND after ? 'ai.price_sync'`).Scan(&n)
	}); err != nil {
		t.Fatalf("counting the switch's audit rows: %v", err)
	}
	if n != 1 {
		t.Errorf("%d audit rows for the switch, want 1", n)
	}
}
