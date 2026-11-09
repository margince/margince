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

// allCounters are the counts a role can hold a value for.
var allCounters = []agentvolume.Counter{agentvolume.Writes, agentvolume.Egress, agentvolume.Calls}

// saveRoleCalls saves a calls value for role as the admin, and gives it up
// when the test ends: the settings are the installation's.
func (e *revocationEnv) saveRoleCalls(t *testing.T, role string, calls int) {
	t.Helper()
	e.patchRoles(t, RoleAllowanceSet{RoleKey: role, Set: map[agentvolume.Counter]int{agentvolume.Calls: calls}})
	t.Cleanup(func() {
		sets := []RoleAllowanceSet{{RoleKey: role, Unset: allCounters}}
		if _, err := e.allowanceSettings().UpdateInstallation(e.wsCtx(e.admin),
			InstallationPatch{AgentAllowances: AgentAllowancePatch{Roles: &sets}}); err != nil {
			t.Errorf("giving up %s's values: %v", role, err)
		}
	})
}

func (e *revocationEnv) patchRoles(t *testing.T, sets ...RoleAllowanceSet) {
	t.Helper()
	if _, err := e.allowanceSettings().UpdateInstallation(e.wsCtx(e.admin),
		InstallationPatch{AgentAllowances: AgentAllowancePatch{Roles: &sets}}); err != nil {
		t.Fatalf("saving role values: %v", err)
	}
}

// roleMeter is a meter over this env's saved allowances, on a stopped clock.
func (e *revocationEnv) roleMeter(t *testing.T) *agentvolume.Meter {
	t.Helper()
	at := clocktest.Now(t)
	return agentvolume.NewWithClock(redistest.Client(t), agentvolume.Limits{}, agentvolume.DefaultWindow,
		func() time.Time { return at }).WithPolicySource(storedPolicy{e})
}

// agentOn is the call context of the agent a token authenticates to now.
func (e *revocationEnv) agentOn(t *testing.T, token string) context.Context {
	t.Helper()
	agent, err := e.svc.AuthenticateAgent(e.wsCtx(e.member), token)
	if err != nil {
		t.Fatalf("authenticating the passport: %v", err)
	}
	return principal.WithActor(principal.WithWorkspaceID(context.Background(), e.ws.UUID), agent.Principal())
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
	e.saveRoleCalls(t, custom, 5000)
	meter := e.roleMeter(t)
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

// Admin taken away through the member screens lowers the limit on the next
// call of a passport the member already holds. The count it spent stays.
func TestLosingAdminLowersARealPassportsLimitAndKeepsItsCount(t *testing.T) {
	e := setupRevocationEnv(t, "role-allowance-admin")
	if err := e.svc.ChangeUserRole(e.wsCtx(e.admin), e.admin, e.member.UserID, adminRoleKey); err != nil {
		t.Fatalf("making the member an Admin: %v", err)
	}
	meter := e.roleMeter(t)
	issued, err := e.svc.IssuePassport(e.wsCtx(e.member), e.resolve(t, e.member.UserID), IssuePassportInput{Scopes: []string{"read"}})
	if err != nil {
		t.Fatalf("minting a passport: %v", err)
	}
	if err := meter.Consume(e.agentOn(t, issued.Token), agentvolume.Calls, 1500); err != nil {
		t.Fatal(err)
	}
	if r := meter.Read(e.agentOn(t, issued.Token), agentvolume.Calls); r.Exceeded || r.Limit != 20000 {
		t.Fatalf("as an Admin the passport reads %+v, want 1500 inside the preset's 20000", r)
	}

	if err := e.svc.ChangeUserRole(e.wsCtx(e.admin), e.admin, e.member.UserID, "rep"); err != nil {
		t.Fatalf("taking Admin away: %v", err)
	}

	r := meter.Read(e.agentOn(t, issued.Token), agentvolume.Calls)
	if !r.Exceeded || r.Observed != 1500 || r.Limit != agentvolume.DefaultCalls {
		t.Errorf("once a User the passport reads %+v, want its 1500 kept against %d and refused", r, agentvolume.DefaultCalls)
	}
}

// A connected app's renewed passport counts on where the last one stopped, and
// against the limit its human's role gives.
func TestARenewedPassportKeepsItsCountUnderARoleLimit(t *testing.T) {
	e := setupRevocationEnv(t, "role-allowance-renewal")
	if err := e.svc.ChangeUserRole(e.wsCtx(e.admin), e.admin, e.member.UserID, "rep"); err != nil {
		t.Fatalf("making the member a User: %v", err)
	}
	e.saveRoleCalls(t, "rep", 300)
	meter := e.roleMeter(t)
	fixture := e.connectOAuthFor(t, e.member)
	before := e.rotateIssuing(t, &fixture)
	if err := meter.Consume(e.agentOn(t, before.Token), agentvolume.Calls, 250); err != nil {
		t.Fatal(err)
	}

	after := e.rotateIssuing(t, &fixture)

	r := meter.Read(e.agentOn(t, after.Token), agentvolume.Calls)
	if r.Observed != 250 || r.Limit != 300 || r.Exceeded {
		t.Errorf("the renewed passport reads %+v, want the 250 already spent against the role's 300", r)
	}
}

// A save of other roles leaves an archived role's value alone, so restoring
// the role brings its value back with it.
func TestAnArchivedRolesValueOutlivesASaveAndComesBackOnRestore(t *testing.T) {
	e := setupRevocationEnv(t, "role-allowance-restore")
	custom := e.customRole(t, "Seasonal research", "rep", nil)
	e.saveRoleCalls(t, custom, 4000)
	if _, err := e.svc.ArchiveRole(e.wsCtx(e.admin), e.admin, custom); err != nil {
		t.Fatalf("archiving the role: %v", err)
	}

	e.saveRoleCalls(t, "rep", 2000)
	if _, err := e.svc.RestoreRole(e.wsCtx(e.admin), e.admin, custom); err != nil {
		t.Fatalf("restoring the role: %v", err)
	}

	listed, err := e.allowanceSettings().GetInstallation(e.wsCtx(e.admin))
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range listed.AgentRoleAllowances {
		if role.Key == custom && role.Own.Calls != 4000 {
			t.Errorf("the restored role holds %+v, want the 4000 it was saved with", role.Own)
		}
	}
}
