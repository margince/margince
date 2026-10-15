// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/identity"
)

// A value the api's environment sets wins over the database and refuses a
// change, through the same wiring cmd/api boots.
func TestSendRateLimitSetByEnvIsLockedAgainstAChange(t *testing.T) {
	e := apptest.SetupAppWithEnv(t, []string{"MARGINCE_INSTALLATION_SEND_RATE_LIMIT=7"})
	e.BootstrapWorkspace(t)

	var read struct {
		Operations struct {
			SendRateLimit   int `json:"send_rate_limit"`
			SendMaxAgeHours int `json:"send_max_age_hours"`
		} `json:"operations"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/installation/settings", nil, nil, &read); status != http.StatusOK {
		t.Fatalf("GET /installation/settings → %d", status)
	}
	if read.Operations.SendRateLimit != 7 {
		t.Fatalf("send_rate_limit reads %d, want the 7 the environment sets", read.Operations.SendRateLimit)
	}
	if status := e.Call(t, http.MethodPatch, "/v1/installation/settings",
		map[string]any{"send_rate_limit": 8}, nil, nil); status != http.StatusConflict {
		t.Fatalf("a change of the locked send_rate_limit → %d, want 409", status)
	}
	if status := e.Call(t, http.MethodPatch, "/v1/installation/settings",
		map[string]any{"send_rate_limit": 7, "send_max_age_hours": 12}, nil, nil); status != http.StatusOK {
		t.Fatalf("a form resending the locked value beside a change → %d, want 200", status)
	}
	if status := e.Call(t, http.MethodGet, "/v1/installation/settings", nil, nil, &read); status != http.StatusOK ||
		read.Operations.SendMaxAgeHours != 12 {
		t.Fatalf("after the save: %d, send_max_age_hours %d, want 12 saved", status, read.Operations.SendMaxAgeHours)
	}
	var rows int
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM setting WHERE key = $1`, identity.SendRateLimit.Key()).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("the locked setting wrote %d rows, want none", rows)
	}
}
