// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestACheckReadsBackAsItIsWritten(t *testing.T) {
	for _, name := range []string{"deal.update", "deal.create|update", "*.create", "admin", "scope:team"} {
		c, err := ParseCheck(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if c.String() != name {
			t.Errorf("%s reads back as %s", name, c)
		}
	}
	for _, bad := range []string{"", "deal", "deal.", ".read", "deal.archive", "scope:everyone"} {
		if _, err := ParseCheck(bad); err == nil {
			t.Errorf("%q reads as a check", bad)
		}
	}
}

func TestHeldByIsTheGatesDecision(t *testing.T) {
	rep := principal.Principal{Type: principal.PrincipalAgent, ID: "agent:rep", Permissions: principal.Permissions{
		RoleKeys: []string{"rep"}, Objects: map[string]principal.ObjectGrant{"deal": {Read: true, Update: true}},
	}}
	admin := rep
	admin.Permissions.RoleKeys = []string{"rep", roleAdmin}
	admin.Permissions.RowScope = principal.RowScopeAll
	for _, tc := range []struct {
		check string
		who   principal.Principal
		want  bool
	}{
		{"deal.update", rep, true},
		{"deal.delete", rep, false},
		{"deal.delete|update", rep, true},
		{"*.update", rep, true},
		{"*.create", rep, false},
		{"admin", rep, false},
		{"admin", admin, true},
		{"scope:team", rep, false},
		{"scope:team", admin, true},
		{"deal.read", principal.Principal{Type: principal.PrincipalBuyer}, false},
		{"admin", principal.Principal{Type: principal.PrincipalSystem}, true},
	} {
		c, err := ParseCheck(tc.check)
		if err != nil {
			t.Fatal(err)
		}
		ctx := principal.WithActor(context.Background(), tc.who)
		if got := c.HeldBy(tc.who); got != tc.want {
			t.Errorf("%s held by %s = %v, want %v", tc.check, tc.who.ID, got, tc.want)
		}
		if gate := gateAdmits(ctx, c); gate != tc.want {
			t.Errorf("%s: the gate admits %s = %v, and HeldBy says %v", tc.check, tc.who.ID, gate, tc.want)
		}
	}
}

// gateAdmits asks the gate that decides c.
func gateAdmits(ctx context.Context, c Check) bool {
	switch {
	case c.Admin:
		return RequireAdmin(ctx) == nil
	case c.RowScope != "":
		return RequireRowScope(ctx, c.RowScope) == nil
	case c.Object == anyObject:
		return RequireAnyObject(ctx, c.Actions[0]) == nil
	}
	return RequireAny(ctx, c.Object, c.Actions...) == nil
}

// A gate records the checks it asks of the caller; a branch, and a check asked
// of another principal, record nothing.
func TestOnlyARequirementOfTheCallerIsRecorded(t *testing.T) {
	caller := principal.Principal{Type: principal.PrincipalAgent, ID: "agent:caller", Permissions: principal.Permissions{
		Objects: map[string]principal.ObjectGrant{"deal": {Read: true}},
	}}
	rec := &checkRecorder{actor: caller.ID, checks: map[string]bool{}}
	ctx := context.WithValue(principal.WithActor(context.Background(), caller), checkRecorderKey{}, rec)

	if err := Require(ctx, "deal", principal.ActionRead); err != nil {
		t.Fatal(err)
	}
	if RequireAny(ctx, "contact", principal.ActionCreate, principal.ActionUpdate) == nil || RequireAdmin(ctx) == nil {
		t.Fatal("the caller holds no contact grant and no admin role, and was admitted")
	}
	if Allows(ctx, "deal", principal.ActionDelete) || AdminGranted(ctx) {
		t.Fatal("a branch answered yes to a grant the caller lacks")
	}
	// The caller holds neither delete nor update, so the archive is refused
	// before it reads a row.
	if EnsureArchivable(ctx, pgx.Tx(nil), DealArchive, ids.NewV7()) == nil {
		t.Fatal("the archive was admitted without delete or update")
	}
	colleague := principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalHuman, ID: "user:colleague"})
	if Require(colleague, "lead", principal.ActionRead) == nil {
		t.Fatal("a colleague with no grant was admitted")
	}

	got := make([]string, 0, len(rec.checks))
	for name := range rec.checks {
		got = append(got, name)
	}
	slices.Sort(got)
	want := []string{"admin", "contact.create|update", "deal.read"}
	if !slices.Equal(got, want) {
		t.Errorf("recorded %v, want %v", got, want)
	}
}
