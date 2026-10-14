// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// patchAllowances sends one PATCH as ctx and returns the status and the
// allowances the response carries.
func patchAllowances(t *testing.T, ctx context.Context, h installationSettingsHandlers, body string) (int, crmcontracts.AgentAllowanceSettings) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/v1/installation/settings", strings.NewReader(body)).WithContext(ctx)
	rec := httptest.NewRecorder()
	h.UpdateInstallationSettings(rec, req)
	var out crmcontracts.InstallationSettings
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decoding the PATCH answer: %v", err)
		}
	}
	return rec.Code, out.AgentAllowances
}

const defaultAgentAllowances = `{"agent_reads_per_window":2000,"agent_writes_per_window":200,` +
	`"agent_egress_per_window":20,"agent_calls_per_window":1000,"agent_window_hours":24}`

// allowanceHandlers builds the surface on a store whose clock reads now(). It
// puts the defaults back when the test ends, because the settings are
// installation-wide and the database is shared.
func allowanceHandlers(t *testing.T, e *integration.Env, now func() time.Time) installationSettingsHandlers {
	t.Helper()
	store := identity.NewInstallationSettings(e.DB(), NewSettingsStore(e.Pool)).WithClock(now)
	h := installationSettingsHandlers{store: store}
	t.Cleanup(func() {
		if code, _ := patchAllowances(t, installationAdminCtx(e), h, defaultAgentAllowances); code != http.StatusOK {
			t.Errorf("restoring the defaults → %d", code)
		}
	})
	return h
}

func TestAnAdminSavesTheAgentAllowances(t *testing.T) {
	e := integration.Setup(t)
	h := allowanceHandlers(t, e, time.Now)

	code, saved := patchAllowances(t, installationAdminCtx(e), h,
		`{"agent_reads_per_window":500,"agent_writes_per_window":10,"agent_egress_per_window":3,"agent_calls_per_window":60}`)
	if code != http.StatusOK {
		t.Fatalf("saving the allowances → %d", code)
	}
	if saved.AgentReadsPerWindow != 500 || saved.AgentWritesPerWindow != 10 ||
		saved.AgentEgressPerWindow != 3 || saved.AgentCallsPerWindow != 60 {
		t.Fatalf("the save answered %+v, want the values sent", saved)
	}

}

func TestAWindowChangeWaitsForTheNextReset(t *testing.T) {
	e := integration.Setup(t)
	clock := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	h := allowanceHandlers(t, e, func() time.Time { return clock })

	code, saved := patchAllowances(t, installationAdminCtx(e), h, `{"agent_window_hours":12}`)
	if code != http.StatusOK {
		t.Fatalf("saving a window length → %d", code)
	}
	policy, err := readAgentPolicyAsMachinery(e)
	if err != nil {
		t.Fatal(err)
	}
	midnight := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	if policy.Window.Length != 24*time.Hour || policy.Window.Next != 12*time.Hour || !policy.Window.NextFrom.Equal(midnight) {
		t.Fatalf("the stored plan is %+v, want 24h until %v then 12h", policy.Window, midnight)
	}
	if saved.AgentWindowNextHours == nil || *saved.AgentWindowNextHours != 12 || saved.AgentWindowNextStartsAt == nil {
		t.Errorf("the answer does not report the waiting change: %+v", saved)
	}

	code, _ = patchAllowances(t, installationAdminCtx(e), h, `{"agent_window_hours":24}`)
	if code != http.StatusOK {
		t.Fatalf("cancelling the change → %d", code)
	}
	if policy, err = readAgentPolicyAsMachinery(e); err != nil {
		t.Fatal(err)
	}
	if policy.Window.Next != 0 {
		t.Errorf("sending the length in force left a change waiting: %+v", policy.Window)
	}
}

func TestAnAgentAllowanceOutsideItsRangeIsRefused(t *testing.T) {
	e := integration.Setup(t)
	h := allowanceHandlers(t, e, time.Now)
	for _, body := range []string{
		`{"agent_reads_per_window":99}`,
		`{"agent_writes_per_window":5001}`,
		`{"agent_egress_per_window":0}`,
		`{"agent_calls_per_window":49}`,
		`{"agent_window_hours":169}`,
	} {
		if code, _ := patchAllowances(t, installationAdminCtx(e), h, body); code != http.StatusUnprocessableEntity {
			t.Errorf("PATCH %s → %d, want 422", body, code)
		}
	}
}

func TestAnAgentCannotChangeItsOwnAllowance(t *testing.T) {
	e := integration.Setup(t)
	h := allowanceHandlers(t, e, time.Now)
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:" + ids.NewV7().String(), PassportID: ids.NewV7(),
		OnBehalfOf: ids.NewV7(),
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"installation_settings": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})

	if code, _ := patchAllowances(t, ctx, h, `{"agent_writes_per_window":5000}`); code != http.StatusForbidden {
		t.Fatalf("an agent raising its own allowance → %d, want 403", code)
	}
}

// readAgentPolicyAsMachinery reads the plan the way the meter does, so the
// test sees what binds rather than what the settings answer reports.
func readAgentPolicyAsMachinery(e *integration.Env) (agentvolume.Policy, error) {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	var policy agentvolume.Policy
	err := NewSettingsStore(e.Pool).WriteTx(ctx, func(tx pgx.Tx) error {
		var err error
		policy, err = identity.AgentPolicyTx(ctx, tx)
		return err
	})
	return policy, err
}
