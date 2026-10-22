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
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/redistest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// patchAllowances sends one PATCH as ctx and returns the status and the
// allowances the response carries.
func patchAllowances(ctx context.Context, t *testing.T, h installationSettingsHandlers, body string) (int, crmcontracts.AgentAllowanceSettings) {
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

const defaultAgentAllowances = `{"agent_writes_per_window":200,` +
	`"agent_egress_per_window":20,"agent_calls_per_window":1000,"agent_window_hours":24}`

// allowanceHandlers builds the surface on a store whose clock reads now(). It
// puts the defaults back when the test ends, because the settings are
// installation-wide and the database is shared.
func allowanceHandlers(t *testing.T, e *integration.Env, now func() time.Time) installationSettingsHandlers {
	t.Helper()
	store := identity.NewInstallationSettings(e.DB(), NewSettingsStore(e.Pool), StoredConfig(e.Pool)).WithClock(now)
	h := installationSettingsHandlers{store: store, now: now}
	t.Cleanup(func() {
		if code, _ := patchAllowances(installationAdminCtx(e), t, h, defaultAgentAllowances); code != http.StatusOK {
			t.Errorf("restoring the defaults → %d", code)
		}
	})
	return h
}

func TestAnAdminSavesTheAgentAllowances(t *testing.T) {
	e := integration.Setup(t)
	h := allowanceHandlers(t, e, wallClock(t))

	code, saved := patchAllowances(installationAdminCtx(e), t, h,
		`{"agent_writes_per_window":10,"agent_egress_per_window":3,"agent_calls_per_window":60}`)
	if code != http.StatusOK {
		t.Fatalf("saving the allowances → %d", code)
	}
	if saved.AgentWritesPerWindow != 10 ||
		saved.AgentEgressPerWindow != 3 || saved.AgentCallsPerWindow != 60 {
		t.Fatalf("the save answered %+v, want the values sent", saved)
	}

	// Each changed allowance leaves its before and after in the audit log.
	var audited int
	if err := e.Pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_log
		WHERE entity_type = 'installation_settings'
		  AND after ->> $1 = '10'`,
		identity.AgentWritesPerWindow.Key()).Scan(&audited); err != nil {
		t.Fatalf("reading the audit log: %v", err)
	}
	if audited == 0 {
		t.Errorf("no audit row records %s becoming 10", identity.AgentWritesPerWindow.Key())
	}
}

func TestAWindowChangeWaitsForTheNextReset(t *testing.T) {
	e := integration.Setup(t)
	clock := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	h := allowanceHandlers(t, e, func() time.Time { return clock })

	code, saved := patchAllowances(installationAdminCtx(e), t, h, `{"agent_window_hours":12}`)
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

	code, _ = patchAllowances(installationAdminCtx(e), t, h, `{"agent_window_hours":24}`)
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
	h := allowanceHandlers(t, e, wallClock(t))
	for _, body := range []string{
		`{"agent_writes_per_window":5001}`,
		`{"agent_egress_per_window":0}`,
		`{"agent_calls_per_window":49}`,
		`{"agent_window_hours":169}`,
	} {
		if code, _ := patchAllowances(installationAdminCtx(e), t, h, body); code != http.StatusUnprocessableEntity {
			t.Errorf("PATCH %s → %d, want 422", body, code)
		}
	}
}

func TestAnAgentCannotChangeItsOwnAllowance(t *testing.T) {
	e := integration.Setup(t)
	h := allowanceHandlers(t, e, wallClock(t))
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:" + ids.NewV7().String(), PassportID: ids.NewV7(),
		OnBehalfOf: ids.NewV7(),
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"installation_settings": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})

	if code, _ := patchAllowances(ctx, t, h, `{"agent_writes_per_window":5000}`); code != http.StatusForbidden {
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
		policy, err = identity.AgentPolicyTx(ctx, tx, loadedConfig())
		return err
	})
	return policy, err
}

// The meter judges an agent by what an admin saved, read through the same
// source the API composes.
func TestAWriteAllowanceSavedByAnAdminBindsTheAgent(t *testing.T) {
	e := integration.Setup(t)
	h := allowanceHandlers(t, e, wallClock(t))
	at := clocktest.Now(t)
	meter := agentvolume.NewWithClock(redistest.Client(t), agentvolume.Limits{}, agentvolume.DefaultWindow,
		func() time.Time { return at }).WithPolicySource(NewAgentAllowanceSource(e.Pool))
	agent := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:importer", PassportID: ids.NewV7(), OnBehalfOf: ids.NewV7(),
	})

	if code, _ := patchAllowances(installationAdminCtx(e), t, h, `{"agent_writes_per_window":10}`); code != http.StatusOK {
		t.Fatalf("saving a write allowance of 10 → %d", code)
	}
	for i := range 10 {
		if _, ok, err := meter.Reserve(agent, agentvolume.Writes, 1); err != nil || !ok {
			t.Fatalf("write %d of 10 was refused (err %v)", i+1, err)
		}
	}
	if _, ok, err := meter.Reserve(agent, agentvolume.Writes, 1); err != nil || ok {
		t.Fatalf("the 11th write → admitted=%v err=%v, want refused", ok, err)
	}

	h.afterSave = meter.ForgetPolicy
	if code, _ := patchAllowances(installationAdminCtx(e), t, h, `{"agent_writes_per_window":20}`); code != http.StatusOK {
		t.Fatalf("raising the allowance to 20 → %d", code)
	}
	if _, ok, err := meter.Reserve(agent, agentvolume.Writes, 1); err != nil || !ok {
		t.Errorf("after the raise the 11th write → admitted=%v err=%v, want admitted", ok, err)
	}
}

// A step-up on a connection's passport releases the connection's counter: the
// passport the agent was refused on may have been renewed since.
func TestAReleaseOfAConnectionsPassportWidensTheConnection(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	passport, grant := seedConnectionPassport(t, e)
	meter := agentvolume.NewWithClock(redistest.Client(t), agentvolume.Limits{Writes: 1}, time.Hour, wallClock(t))
	renewed := principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:connector", PassportID: ids.NewV7(), ConnectionID: grant,
	})
	if err := meter.Consume(renewed, agentvolume.Writes, 1); err != nil {
		t.Fatal(err)
	}

	applied, err := connectionReleaser{meter: meter, pool: e.Pool}.Release(ctx, e.WS, passport, agentvolume.Writes,
		meter.Bucket(ctx), time.Hour)
	if err != nil || !applied {
		t.Fatalf("releasing the connection → applied=%v err=%v", applied, err)
	}

	if reading := meter.Read(renewed, agentvolume.Writes); reading.Exceeded {
		t.Errorf("the renewed passport still reads %+v after its connection was released", reading)
	}
}

// seedConnectionPassport mints a connection for Rep1 and the passport the
// token exchange would issue under it, and returns both ids.
func seedConnectionPassport(t *testing.T, e *integration.Env) (passport, grant ids.UUID) {
	t.Helper()
	ctx := context.Background()
	client, grant, passport := "client-"+ids.NewV7().String(), ids.NewV7(), ids.NewV7()
	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{
			`INSERT INTO oauth_client (client_id, client_name, redirect_uris) VALUES ($1, 'Claude', ARRAY['https://client.example/cb'])`,
			[]any{client},
		},
		{
			`INSERT INTO oauth_grant (id, client_id, user_id, scopes, refresh_allowed) VALUES ($1, $2, $3, ARRAY['read']::text[], true)`,
			[]any{grant, client, e.Rep1},
		},
		{
			`INSERT INTO passport (id, on_behalf_of, granted_by, label, scopes, token_hash, expires_at, oauth_grant_id)
		  VALUES ($1, $2, $2, 'oauth:claude', ARRAY['read']::text[], $3, now() + interval '1 day', $4)`,
			[]any{passport, e.Rep1, "hash-" + passport.String(), grant},
		},
	} {
		if _, err := e.Pool.Exec(ctx, stmt.sql, stmt.args...); err != nil {
			t.Fatalf("seeding the connection: %v", err)
		}
	}
	return passport, grant
}

// wallClock is a store clock pinned to the fixture instant.
func wallClock(t *testing.T) func() time.Time {
	at := clocktest.Now(t)
	return func() time.Time { return at }
}

// A step-up answered after its passport row is gone still decides. The
// passport is counted as itself, as it was if it was not a connection's.
func TestAReleaseOfAGonePassportStillDecides(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	meter := agentvolume.NewWithClock(redistest.Client(t), agentvolume.Limits{Writes: 1}, time.Hour, wallClock(t))

	applied, err := connectionReleaser{meter: meter, pool: e.Pool}.Release(ctx, e.WS, ids.NewV7(), agentvolume.Writes,
		meter.Bucket(ctx), time.Hour)

	if err != nil || !applied {
		t.Fatalf("releasing a passport with no row → applied=%v err=%v, want applied", applied, err)
	}
}
