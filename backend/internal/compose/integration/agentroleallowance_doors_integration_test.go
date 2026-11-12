// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/platform/redistest"
)

// roleBoundDoors is the app under a live meter that reads the saved
// allowances, as the API composes it, with the admin's passport on both doors.
func roleBoundDoors(t *testing.T, slug string, extra ...compose.Option) *apptest.TwoDoors {
	t.Helper()
	meter := agentvolume.New(redistest.Client(t), agentvolume.Limits{}, time.Hour)
	sourced := func(s *compose.Server, pool *pgxpool.Pool) {
		compose.WithAgentVolume(meter.WithPolicySource(compose.NewAgentAllowanceSource(pool)))(s, pool)
	}
	return apptest.SetupTwoDoors(t, slug, append([]compose.Option{sourced}, extra...), "read")
}

type limitRefusal struct {
	Code    string `json:"code"`
	Details struct {
		Limit struct {
			Kind    string `json:"kind"`
			Ceiling int    `json:"ceiling"`
			Used    int    `json:"used"`
		} `json:"limit"`
	} `json:"details"`
}

// An Admin value saved through the settings binds the admin's real passport,
// and its tool calls and REST requests fill one count against it.
func TestARoleLimitBindsOneCountAcrossMCPAndRest(t *testing.T) {
	d := roleBoundDoors(t, "role-limit-doors")
	if status := d.E.Call(t, "PATCH", "/v1/installation/settings", AnyMap{
		"agent_role_allowances": []AnyMap{{"role_key": "admin", "agent_calls_per_window": 60}},
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("saving Admin's calls → %d", status)
	}

	for i := range 30 {
		d.MCP.CallOK(t, "read_workspace", AnyMap{"topic": "saved_views"})
		if status := d.E.Call(t, "GET", "/v1/contacts", nil, d.Bearer, nil); status != http.StatusOK {
			t.Fatalf("REST read %d of 30 → %d", i+1, status)
		}
	}

	var refusal limitRefusal
	status := d.E.Call(t, "GET", "/v1/contacts", nil, d.Bearer, &refusal)
	if status != http.StatusTooManyRequests || refusal.Details.Limit.Kind != "calls" ||
		refusal.Details.Limit.Ceiling != 60 || refusal.Details.Limit.Used != 60 {
		t.Fatalf("call 61 over REST → %d %+v, want 429 naming 60 of 60 calls", status, refusal)
	}
	if text := d.MCP.CallRefused(t, "read_workspace", AnyMap{"topic": "saved_views"}); !strings.Contains(text, fmt.Sprintf("%d of its %d calls", 60, 60)) {
		t.Errorf("call 62 over MCP says %q, want the same 60 of 60 calls", text)
	}
}

// A meter that cannot read its counter refuses an agent's REST read as a short
// outage. It is never a limit reached that would last until the window rolls.
func TestAnUnreachableCounterRefusesARestReadAsAShortWait(t *testing.T) {
	e := apptest.SetupAppWithOptions(t, compose.WithAgentVolume(agentvolume.New(nil, agentvolume.Limits{}, time.Hour)))
	apptest.BootstrapWorkspaceSession(t, e, "Unreachable Counter", "unreachable-counter@fable.test", "Admin")
	bearer, _ := apptest.PassportWithID(t, e, "reading agent", "read")

	var refusal limitRefusal
	status := e.Call(t, "GET", "/v1/contacts", nil, bearer, &refusal)

	if status != http.StatusServiceUnavailable || refusal.Code != "agent_volume_unavailable" || refusal.Details.Limit.Kind != "" {
		t.Errorf("a read with the counter out of reach → %d %+v, want 503 agent_volume_unavailable with no limit", status, refusal)
	}
}
