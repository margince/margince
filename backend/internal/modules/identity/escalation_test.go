// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The administration objects are named through their constants, never as bare
// string literals at a call site.
//
// A misspelt literal is the defect worth a gate: `auth.Require(ctx, "user_admn",
// …)` compiles, resolves to the zero grant, and refuses every caller. In
// production that reads as a feature that stopped working rather than as a typo,
// and the refusal it produces is indistinguishable from a correct one.
//
// The constants make the same mistake a compile error — but only for code that
// uses them, which is why this fails on the literal instead of trusting that
// nobody will write one.
func TestIdentityNamesItsRbacObjectsThroughConstants(t *testing.T) {
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("listing the package: %v", err)
	}
	// The declarations themselves are where the strings belong.
	const owner = "escalation.go"
	literal := regexp.MustCompile(`"(user_admin|role_admin|team_admin)"`)

	var found []string
	for _, name := range sources {
		if name == owner {
			continue
		}
		body, readErr := os.ReadFile(name)
		if readErr != nil {
			t.Fatalf("reading %s: %v", name, readErr)
		}
		for _, hit := range literal.FindAllString(string(body), -1) {
			found = append(found, name+": "+hit)
		}
	}
	if len(found) > 0 {
		t.Errorf("an administration object is named as a bare string literal at %v — use the "+
			"objectUserAdmin/objectRoleAdmin/objectTeamAdmin constants in %s, so a misspelling is a "+
			"compile error rather than a gate that silently refuses everybody", found, owner)
	}

	// The corpus has to contain the declarations, or this scan is reading a
	// package that no longer holds them and reports PASS either way.
	body, err := os.ReadFile(owner)
	if err != nil {
		t.Fatalf("reading %s: %v", owner, err)
	}
	for _, name := range []string{"objectUserAdmin", "objectRoleAdmin", "objectTeamAdmin"} {
		if !regexp.MustCompile(name + `\s*= "`).Match(body) {
			t.Errorf("%s no longer declares %s, so the literals this test forbids have nowhere "+
				"to come from and it is guarding nothing", owner, name)
		}
	}
}

// Containment of a whole seat, one axis at a time. Each case differs from a
// contained target in exactly one way, so a check that stopped reading that
// axis fails its own case.
func TestWholeAccessContainmentReadsEveryAxis(t *testing.T) {
	teamA, teamB := ids.New[ids.TeamKind](), ids.New[ids.TeamKind]()
	mask := principal.FieldMask{Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways}
	caller := Identity{
		Teams: []ids.TeamID{teamA},
		Permissions: principal.Permissions{
			Objects:    map[string]principal.ObjectGrant{"deal": {Read: true, Update: true}},
			RowScope:   principal.RowScopeTeam,
			FieldMasks: []principal.FieldMask{mask},
		},
	}
	contained := func() seatGrants {
		return seatGrants{
			teams: []ids.TeamID{teamA},
			perms: principal.Permissions{
				Objects:    map[string]principal.ObjectGrant{"deal": {Read: true}},
				RowScope:   principal.RowScopeTeam,
				FieldMasks: []principal.FieldMask{mask},
			},
		}
	}
	if !containsWholeAccess(caller, contained()) {
		t.Fatal("a target narrower on every axis is not contained — every refusal below would pass for the wrong reason")
	}
	for name, widen := range map[string]func(*seatGrants){
		"a grant the caller lacks": func(g *seatGrants) { g.perms.Objects["deal"] = principal.ObjectGrant{Delete: true} },
		"a wider row scope":        func(g *seatGrants) { g.perms.RowScope = principal.RowScopeAll },
		"a team the caller is not on": func(g *seatGrants) {
			g.teams = []ids.TeamID{teamB}
		},
		"a mask the caller carries and the target lacks": func(g *seatGrants) { g.perms.FieldMasks = nil },
		"leading a team the caller is not on": func(g *seatGrants) {
			g.teams = []ids.TeamID{teamB}
			g.perms.RowScope = principal.RowScopeOwn
			g.perms.Objects[objectTeamLead] = principal.ObjectGrant{Read: true}
		},
	} {
		target := contained()
		widen(&target)
		if containsWholeAccess(caller, target) {
			t.Errorf("%s: contained, want refused", name)
		}
	}
	// A team outside the caller's is harmless when nothing the target holds
	// follows its teams.
	ownScoped := contained()
	ownScoped.teams = []ids.TeamID{teamB}
	ownScoped.perms.RowScope = principal.RowScopeOwn
	if !containsWholeAccess(caller, ownScoped) {
		t.Error("an own-scoped target on another team was refused; its team gives it nothing")
	}
}

// Only a literal admin widens what a role grants; narrowing is open to every
// holder of the grant, and the delegate's other grants do not substitute.
func TestOnlyAnAdminWidensARole(t *testing.T) {
	delegate := Identity{
		Roles:       []string{"custom_role_editor"},
		Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{objectRoleAdmin: {Read: true, Update: true}}},
	}
	if err := refuseWideningUnlessAdmin(delegate, true); !errors.Is(err, errWideningRequiresAdmin) {
		t.Errorf("a delegate widening: %v, want errWideningRequiresAdmin", err)
	}
	if err := refuseWideningUnlessAdmin(delegate, false); err != nil {
		t.Errorf("a delegate narrowing: %v, want allowed", err)
	}
	if err := refuseWideningUnlessAdmin(Identity{Roles: []string{roleAdmin}}, true); err != nil {
		t.Errorf("an admin widening: %v, want allowed", err)
	}
}
