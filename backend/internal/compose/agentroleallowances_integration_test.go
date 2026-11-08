// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/redistest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// roleRow is the answer's row for one role key.
func roleRow(t *testing.T, saved crmcontracts.AgentAllowanceSettings, key string) crmcontracts.AgentRoleAllowance {
	t.Helper()
	for _, role := range saved.Roles {
		if role.RoleKey == key {
			return role
		}
	}
	t.Fatalf("the answer lists no role %q among %d", key, len(saved.Roles))
	return crmcontracts.AgentRoleAllowance{}
}

func wantValue(t *testing.T, what string, got crmcontracts.AgentRoleAllowanceValue, value int, source crmcontracts.AgentRoleAllowanceValueSource) {
	t.Helper()
	if got.Value != value || got.Source != source {
		t.Errorf("%s is %d from %s, want %d from %s", what, got.Value, got.Source, value, source)
	}
}

// At the shipped values the built-in Admin role gets the preset for calls and
// changes, and sends stay at the installation value. A value an operator
// chose for the installation takes the preset's place for that count only.
func TestTheAdminPresetYieldsToAnInstallationValueSomebodyChose(t *testing.T) {
	e := integration.Setup(t)
	h := allowanceHandlers(t, e, wallClock(t))

	code, saved := patchAllowances(installationAdminCtx(e), t, h, `{}`)
	if code != http.StatusOK {
		t.Fatalf("reading the allowances → %d", code)
	}
	admin := roleRow(t, saved, "admin")
	wantValue(t, "Admin calls", admin.AgentCallsPerWindow, 20000, crmcontracts.AgentRoleAllowanceValueSourceAdminPreset)
	wantValue(t, "Admin changes", admin.AgentWritesPerWindow, 2000, crmcontracts.AgentRoleAllowanceValueSourceAdminPreset)
	wantValue(t, "Admin sends", admin.AgentEgressPerWindow, agentvolume.DefaultEgress, crmcontracts.AgentRoleAllowanceValueSourceInstallation)
	wantValue(t, "User calls", roleRow(t, saved, "rep").AgentCallsPerWindow, agentvolume.DefaultCalls, crmcontracts.AgentRoleAllowanceValueSourceInstallation)

	if code, saved = patchAllowances(installationAdminCtx(e), t, h, `{"agent_calls_per_window":5000}`); code != http.StatusOK {
		t.Fatalf("choosing 5000 calls → %d", code)
	}
	admin = roleRow(t, saved, "admin")
	wantValue(t, "Admin calls after the choice", admin.AgentCallsPerWindow, 5000, crmcontracts.AgentRoleAllowanceValueSourceInstallation)
	wantValue(t, "Admin changes after the choice", admin.AgentWritesPerWindow, 2000, crmcontracts.AgentRoleAllowanceValueSourceAdminPreset)

	policy, err := readAgentPolicyAsMachinery(e)
	if err != nil {
		t.Fatal(err)
	}
	if got := policy.For([]string{"admin"}).Limits; got.Calls != 5000 || got.Writes != 2000 {
		t.Errorf("the meter holds an Admin's agent to %+v, want 5000 calls and 2000 changes", got)
	}
}

// A role's own values bind its users' agents through the meter's real source.
// An agent holding two roles gets the higher of the two for each count.
func TestARoleValueAnAdminSavedBindsThatRolesAgents(t *testing.T) {
	e := integration.Setup(t)
	h := allowanceHandlers(t, e, wallClock(t))
	at := clocktest.Now(t)
	meter := agentvolume.NewWithClock(redistest.Client(t), agentvolume.Limits{}, agentvolume.DefaultWindow,
		func() time.Time { return at }).WithPolicySource(NewAgentAllowanceSource(e.Pool))
	h.afterSave = meter.ForgetPolicy

	code, saved := patchAllowances(installationAdminCtx(e), t, h, `{"agent_role_allowances":[`+
		`{"role_key":"manager","agent_calls_per_window":3000},`+
		`{"role_key":"ops","agent_calls_per_window":2000,"agent_writes_per_window":1000}]}`)
	if code != http.StatusOK {
		t.Fatalf("saving the role values → %d", code)
	}
	wantValue(t, "Team Lead calls", roleRow(t, saved, "manager").AgentCallsPerWindow, 3000, crmcontracts.AgentRoleAllowanceValueSourceRole)
	wantValue(t, "Ops changes", roleRow(t, saved, "ops").AgentWritesPerWindow, 1000, crmcontracts.AgentRoleAllowanceValueSourceRole)
	wantValue(t, "Admin calls", roleRow(t, saved, "admin").AgentCallsPerWindow, 20000, crmcontracts.AgentRoleAllowanceValueSourceAdminPreset)

	both := roledAgent(e, "manager", "ops")
	if r := meter.Read(both, agentvolume.Calls); r.Limit != 3000 {
		t.Errorf("Team Lead and Ops are allowed %d calls, want 3000", r.Limit)
	}
	if r := meter.Read(both, agentvolume.Writes); r.Limit != 1000 {
		t.Errorf("Team Lead and Ops are allowed %d changes, want 1000", r.Limit)
	}
	if r := meter.Read(roledAgent(e, "rep"), agentvolume.Calls); r.Limit != agentvolume.DefaultCalls {
		t.Errorf("a User's agent is allowed %d calls, want the installation's %d", r.Limit, agentvolume.DefaultCalls)
	}

	var audited int
	if err := e.Pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_log
		WHERE entity_type = 'installation_settings' AND after -> $1 ->> 'manager' = '3000'`,
		identity.AgentRoleCallsPerWindow.Key()).Scan(&audited); err != nil {
		t.Fatalf("reading the audit log: %v", err)
	}
	if audited == 0 {
		t.Error("no audit row records Team Lead's calls becoming 3000")
	}
}

// Admin sent with zeros keeps the installation value; unset, it returns to the
// preset.
func TestAdminSentWithZeroUsesTheInstallationValue(t *testing.T) {
	e := integration.Setup(t)
	h := allowanceHandlers(t, e, wallClock(t))

	code, saved := patchAllowances(installationAdminCtx(e), t, h, `{"agent_role_allowances":[`+
		`{"role_key":"admin","agent_calls_per_window":0,"agent_writes_per_window":0}]}`)
	if code != http.StatusOK {
		t.Fatalf("saving Admin at zero → %d", code)
	}
	calls := roleRow(t, saved, "admin").AgentCallsPerWindow
	wantValue(t, "Admin calls", calls, agentvolume.DefaultCalls, crmcontracts.AgentRoleAllowanceValueSourceInstallationKept)
	if calls.IfUnset.Value != 20000 || calls.IfUnset.Source != crmcontracts.AgentRoleAllowanceFallbackSourceAdminPreset {
		t.Errorf("Admin calls would fall back to %+v once unset, want the 20000 preset", calls.IfUnset)
	}

	if code, saved = patchAllowances(installationAdminCtx(e), t, h,
		`{"agent_role_allowances":[{"role_key":"admin","unset":["agent_calls_per_window"]}]}`); code != http.StatusOK {
		t.Fatalf("unsetting Admin's calls → %d", code)
	}
	wantValue(t, "Admin calls once unset", roleRow(t, saved, "admin").AgentCallsPerWindow, 20000, crmcontracts.AgentRoleAllowanceValueSourceAdminPreset)
	wantValue(t, "Admin changes, still saved at zero", roleRow(t, saved, "admin").AgentWritesPerWindow, agentvolume.DefaultWrites, crmcontracts.AgentRoleAllowanceValueSourceInstallationKept)
}

// The preset binds changes while an operator chose calls. Saving another role
// leaves Admin as it was: nothing is saved for a role the save does not name.
func TestSavingAnotherRoleLeavesAPartlyAppliedPresetBinding(t *testing.T) {
	e := integration.Setup(t)
	h := allowanceHandlers(t, e, wallClock(t))
	if code, _ := patchAllowances(installationAdminCtx(e), t, h, `{"agent_calls_per_window":5000}`); code != http.StatusOK {
		t.Fatalf("choosing 5000 calls → %d", code)
	}

	code, saved := patchAllowances(installationAdminCtx(e), t, h,
		`{"agent_role_allowances":[{"role_key":"manager","agent_calls_per_window":6000}]}`)
	if code != http.StatusOK {
		t.Fatalf("saving Team Lead → %d", code)
	}

	admin := roleRow(t, saved, "admin")
	wantValue(t, "Admin changes", admin.AgentWritesPerWindow, 2000, crmcontracts.AgentRoleAllowanceValueSourceAdminPreset)
	wantValue(t, "Admin calls", admin.AgentCallsPerWindow, 5000, crmcontracts.AgentRoleAllowanceValueSourceInstallation)
}

// A default somebody saved is a choice: Admin keeps it rather than the preset.
func TestAnInstallationValueSavedAtTheDefaultKeepsThePresetOff(t *testing.T) {
	e := integration.Setup(t)
	h := allowanceHandlers(t, e, wallClock(t))
	for _, body := range []string{`{"agent_calls_per_window":5000}`, `{"agent_calls_per_window":1000}`} {
		if code, _ := patchAllowances(installationAdminCtx(e), t, h, body); code != http.StatusOK {
			t.Fatalf("PATCH %s → %d", body, code)
		}
	}

	_, saved := patchAllowances(installationAdminCtx(e), t, h, `{}`)

	wantValue(t, "Admin calls", roleRow(t, saved, "admin").AgentCallsPerWindow, agentvolume.DefaultCalls, crmcontracts.AgentRoleAllowanceValueSourceInstallation)
}

func TestARoleSaveRefusesWhatWouldBindNobody(t *testing.T) {
	e := integration.Setup(t)
	h := allowanceHandlers(t, e, wallClock(t))
	for name, body := range map[string]string{
		"a role that is not live": `{"agent_role_allowances":[{"role_key":"custom_gone","agent_calls_per_window":100}]}`,
		"a role given twice": `{"agent_role_allowances":[` +
			`{"role_key":"rep","agent_calls_per_window":100},` +
			`{"role_key":"rep","agent_calls_per_window":200}]}`,
		"a value below the range":    `{"agent_role_allowances":[{"role_key":"rep","agent_calls_per_window":49}]}`,
		"a count both set and unset": `{"agent_role_allowances":[{"role_key":"rep","agent_calls_per_window":100,"unset":["agent_calls_per_window"]}]}`,
	} {
		if code, _ := patchAllowances(installationAdminCtx(e), t, h, body); code != http.StatusUnprocessableEntity {
			t.Errorf("%s → %d, want 422", name, code)
		}
	}
}

// A user without the installation grant cannot set a role's values. No agent
// can either, even one whose human could: it would raise its own allowance.
func TestOnlyAnAdministratorsOwnSessionSetsRoleValues(t *testing.T) {
	e := integration.Setup(t)
	h := allowanceHandlers(t, e, wallClock(t))
	body := `{"agent_role_allowances":[{"role_key":"rep","agent_calls_per_window":20000}]}`

	reader := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(), UserID: ids.NewV7(), SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects:  map[string]principal.ObjectGrant{"installation_settings": {Read: true}}, RowScope: principal.RowScopeOwn,
		},
	})
	if code, _ := patchAllowances(reader, t, h, body); code != http.StatusForbidden {
		t.Errorf("a User raising their role's allowance → %d, want 403", code)
	}

	agent := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:" + ids.NewV7().String(), PassportID: ids.NewV7(),
		OnBehalfOf: ids.NewV7(), SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			Objects:  map[string]principal.ObjectGrant{"installation_settings": {Read: true, Update: true}}, RowScope: principal.RowScopeAll,
		},
	})
	if code, _ := patchAllowances(agent, t, h, body); code != http.StatusForbidden {
		t.Errorf("an Admin's agent raising an allowance → %d, want 403", code)
	}
}

// roledAgent is a call by an agent whose granting human holds roles.
func roledAgent(e *integration.Env, roles ...string) context.Context {
	return principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:" + ids.NewV7().String(), PassportID: ids.NewV7(),
		Permissions: principal.Permissions{RoleKeys: roles},
	})
}
