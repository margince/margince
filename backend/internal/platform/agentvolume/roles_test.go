// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agentvolume

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// rolePolicy is an installation at the defaults with an Admin value for calls
// and changes, and two roles that each raise one counter.
func rolePolicy() Policy {
	return Policy{
		Limits: Limits{}.withDefaults(),
		Roles: map[string]Limits{
			"admin":   {Calls: 20000, Writes: 2000},
			"manager": {Calls: 3000},
			"ops":     {Calls: 2000, Writes: 1000},
			"guest":   {Calls: 100},
		},
		Window: WindowPlan{Length: DefaultWindow},
	}
}

func TestAnAdminsAgentIsAllowedMoreThanAnOrdinaryUsers(t *testing.T) {
	p := rolePolicy()
	admin, rep := p.For([]string{"admin"}), p.For([]string{"rep"})
	if admin.Limits != (Limits{Calls: 20000, Writes: 2000, Egress: DefaultEgress}) {
		t.Errorf("an Admin's agent is allowed %+v", admin.Limits)
	}
	if rep.Limits != (Limits{}).withDefaults() {
		t.Errorf("a role with no values of its own is allowed %+v, want the installation value", rep.Limits)
	}
	if admin.From[Calls] != "admin" || admin.From[Egress] != "" {
		t.Errorf("the Admin allowance names %v as its sources, want admin for calls and the installation for sends", admin.From)
	}
}

func TestEachCounterTakesTheHighestOfTheUsersRoles(t *testing.T) {
	got := rolePolicy().For([]string{"ops", "manager"})
	if got.Limits != (Limits{Calls: 3000, Writes: 1000, Egress: DefaultEgress}) {
		t.Fatalf("Team Lead and Ops together are allowed %+v", got.Limits)
	}
	if got.From[Calls] != "manager" || got.From[Writes] != "ops" || got.From[Egress] != "" {
		t.Errorf("the sources are %v, want calls from manager, changes from ops, sends from the installation", got.From)
	}
}

// A role set below the installation value lowers only a user it is the whole
// allowance of: any other role they hold counts as the installation value.
func TestALowerRoleValueBindsOnlyWhereNoOtherRoleRaisesIt(t *testing.T) {
	p := rolePolicy()
	if got := p.For([]string{"guest"}).Limits.Calls; got != 100 {
		t.Errorf("a guest alone is allowed %d calls, want 100", got)
	}
	both := p.For([]string{"guest", "rep"})
	if both.Limits.Calls != DefaultCalls || both.From[Calls] != "" {
		t.Errorf("a guest who is also a User is allowed %d calls from %q, want the installation's %d",
			both.Limits.Calls, both.From[Calls], DefaultCalls)
	}
}

func TestAUserWithNoLiveRoleGetsTheInstallationValue(t *testing.T) {
	if got := rolePolicy().For(nil).Limits; got != (Limits{}).withDefaults() {
		t.Errorf("a user with no role is allowed %+v", got)
	}
}

// agentOf is a call by an agent whose granting human holds roles.
func agentOf(ctx context.Context, roles ...string) context.Context {
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:reader",
		Permissions: principal.Permissions{RoleKeys: roles},
	})
}

// The meter holds one policy for every caller. Each call is judged by its own
// caller's roles, so an ordinary user's call right after an Admin's gets the
// ordinary allowance, and a human's call is not judged by any role at all.
func TestTheHeldPolicyNeverLendsOneCallersAllowanceToTheNext(t *testing.T) {
	at := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	src := &scriptedSource{policy: rolePolicy()}
	meter := sourcedMeter(src, &at)
	ctx := inWorkspace(t)

	admin := meter.window(agentOf(ctx, "admin"))
	rep := meter.window(agentOf(ctx, "rep"))
	human := meter.window(principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, Permissions: principal.Permissions{RoleKeys: []string{"admin"}},
	}))

	if src.reads != 1 {
		t.Fatalf("three calls read the source %d times, want once", src.reads)
	}
	if admin.limits.Calls != 20000 || rep.limits.Calls != DefaultCalls {
		t.Errorf("an Admin's agent then a User's are allowed %d and %d calls, want 20000 and %d",
			admin.limits.Calls, rep.limits.Calls, DefaultCalls)
	}
	if human.limits.Calls != DefaultCalls {
		t.Errorf("a human's call reads %d calls; the roles choose an agent's allowance only", human.limits.Calls)
	}
}

func TestARefusalSaysWhenItsWindowResets(t *testing.T) {
	at := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	meter := sourcedMeter(&scriptedSource{policy: rolePolicy()}, &at)

	r := meter.window(inWorkspace(t)).reading(Calls)

	if want := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC); !r.ResetsAt.Equal(want) || r.ResetsIn != 14*time.Hour {
		t.Errorf("the reading resets at %v in %v, want %v in 14h", r.ResetsAt, r.ResetsIn, want)
	}
}
