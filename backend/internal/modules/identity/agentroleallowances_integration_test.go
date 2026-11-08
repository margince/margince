// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/redistest"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// storedPolicy answers the meter with the allowances as AgentPolicyTx reads
// them from this env's settings, the reader the server's source calls.
type storedPolicy struct{ e *revocationEnv }

func (s storedPolicy) Policy(ctx context.Context) (agentvolume.Policy, error) {
	var policy agentvolume.Policy
	err := database.WithWorkspaceTx(ctx, s.e.pool, func(tx pgx.Tx) error {
		var err error
		policy, err = AgentPolicyTx(ctx, tx, s.e.svc.config)
		return err
	})
	return policy, err
}

// allowanceSettings is the installation settings store over this env.
func (e *revocationEnv) allowanceSettings() *InstallationSettingsStore {
	reg := settings.NewRegistry(append(Definitions(), e.svc.config.StoredDefinitions()...)...)
	return NewInstallationSettings(e.svc.db, settings.New(e.pool, reg), e.svc.config)
}

// saveRoleValues replaces every role's values as the admin, and puts them back
// when the test ends: the settings are the installation's.
func (e *revocationEnv) saveRoleValues(t *testing.T, sets ...RoleAllowanceSet) {
	t.Helper()
	store := e.allowanceSettings()
	if _, err := store.UpdateInstallation(e.wsCtx(e.admin), InstallationPatch{AgentAllowances: AgentAllowancePatch{Roles: &sets}}); err != nil {
		t.Fatalf("saving the role values: %v", err)
	}
	t.Cleanup(func() {
		if _, err := store.UpdateInstallation(e.wsCtx(e.admin), InstallationPatch{AgentAllowances: AgentAllowancePatch{Roles: &[]RoleAllowanceSet{}}}); err != nil {
			t.Errorf("clearing the role values: %v", err)
		}
	})
}

// callsAllowed is the calls ceiling the meter holds passport to, as
// authentication resolves its human now.
func (e *revocationEnv) callsAllowed(t *testing.T, meter *agentvolume.Meter, passport ids.PassportID) int {
	t.Helper()
	agent, err := e.svc.AuthenticateAgentByID(e.wsCtx(e.member), passport)
	if err != nil {
		t.Fatalf("authenticating the passport: %v", err)
	}
	ctx := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.ws.UUID), agent.Principal())
	return meter.Read(ctx, agentvolume.Calls).Limit
}

// A custom role takes part in the highest-of-the-roles rule like a seeded one,
// by its key. Once the user no longer holds it and it is archived, its saved
// value binds nobody, and having one did not stop the archive.
func TestACustomRolesHigherValueBindsItsHolderUntilTheRoleGoes(t *testing.T) {
	e := setupRevocationEnv(t, "role-allowance")
	custom := e.customRole(t, "Field research", "rep", nil)
	if err := e.svc.ChangeUserRole(e.wsCtx(e.admin), e.admin, e.member.UserID, "rep"); err != nil {
		t.Fatalf("making the member a User: %v", err)
	}
	// A directory group adds a role on top of the one assigned; this is that grant.
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO role_assignment (role_id, user_id) SELECT id, $2 FROM role WHERE key = $1`, custom, e.member.UserID); err != nil {
		t.Fatal(err)
	}
	e.saveRoleValues(t, RoleAllowanceSet{RoleKey: custom, Values: agentvolume.Limits{Calls: 5000}})
	at := clocktest.Now(t)
	meter := agentvolume.NewWithClock(redistest.Client(t), agentvolume.Limits{}, agentvolume.DefaultWindow,
		func() time.Time { return at }).WithPolicySource(storedPolicy{e})
	passport := e.mintOwnPassport(t, e.resolve(t, e.member.UserID), []string{"read"})

	if got := e.callsAllowed(t, meter, passport); got != 5000 {
		t.Fatalf("a User who also holds Field research is allowed %d calls, want the custom role's 5000", got)
	}

	if err := e.svc.ChangeUserRole(e.wsCtx(e.admin), e.admin, e.member.UserID, "rep"); err != nil {
		t.Fatalf("taking Field research away: %v", err)
	}
	if _, err := e.svc.ArchiveRole(e.wsCtx(e.admin), e.admin, custom); err != nil {
		t.Fatalf("archiving a role with a saved allowance: %v", err)
	}
	if got := e.callsAllowed(t, meter, passport); got != agentvolume.DefaultCalls {
		t.Errorf("after the role went the passport is allowed %d calls, want the installation's %d", got, agentvolume.DefaultCalls)
	}
	listed, err := e.allowanceSettings().GetInstallation(e.wsCtx(e.admin))
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range listed.AgentRoleAllowances {
		if role.Key == custom {
			t.Errorf("the archived role is still listed with %+v", role)
		}
	}
}
