// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The harness's admin is the seat a suite acts through when the seat is not
// what it is testing, and one narrower than production's admin does not fail —
// it answers.
//
// AdminPerms carried no `tag` grant while AccountRepPerms, a REP fixture,
// carried tag.read. Nothing erred: the tag filter refuses a caller without
// tag.read by rendering FALSE (storekit.TagFilterClause — an error would tell
// an outsider that the tag exists), so the admin asked "who carries VIP" and
// was told "nobody", and the suite read that as the store's answer.
//
// So the claim here is the one harnessperms.go already makes in prose — "the
// harness's admin stands in for every seat in most suites" — and the check is
// its contrapositive: no fixture for a seat the SEED grants strictly less than
// admin may hold a grant AdminPerms lacks. Seats, not fixtures. A fixture is
// often narrower than its own seat on purpose, and several here say so where
// they are declared; what cannot happen is the widest stand-in being narrower
// than something it stands in for.
//
// It compares against migrations/testdata/rbac_seeded_defaults.json, which is
// policy's own value: policy sits behind identity/internal/, and
// identity/rbacfixture_test.go lives inside that fence and pins the file to
// policy.MustDefaultJSON on every unit pass.
//
// WHAT IT DOES NOT REACH: a seat the seed grants exactly what it grants admin.
// ops holds the admin grid, so OpsPerms is never compared, and neither is
// AdminWithSignals — an admin fixture carrying a signal grant AdminPerms
// lacks. That narrowing is deliberate and documented where it is declared, but
// this gate is not what keeps it so; nothing is.

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"sort"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// harnessPermsFile is the file whose fixtures this gate governs. The census
// below reads it rather than trusting the map: a fixture added to it and not
// registered here would otherwise be exempt by omission.
const harnessPermsFile = "harnessperms.go"

// adminFixture is the fixture the harness reaches for when a suite wants a
// seat that is not what it is testing. harnessperms.go says so where it grants
// weekly_plan: "the harness's admin stands in for every seat in most suites".
const adminFixture = "AdminPerms"

// harnessSeatFixtures is every permission fixture harnessperms.go declares, by
// name.
//
// Held by: TestEverySeatFixtureIsRegisteredWithTheParityGate
// (backend/internal/compose/integration/harnesspermsparity_integration_test.go)
var harnessSeatFixtures = map[string]principal.Permissions{
	"RepPerms":         RepPerms,
	"ContractRepPerms": ContractRepPerms,
	"AccountRepPerms":  AccountRepPerms,
	"ReadOnlyPerms":    ReadOnlyPerms,
	"OpsPerms":         OpsPerms,
	"AdminWithSignals": AdminWithSignals,
	"AdminPerms":       AdminPerms,
}

func TestEverySeatFixtureIsRegisteredWithTheParityGate(t *testing.T) {
	declared := declaredSeatFixtures(t)
	for _, name := range declared {
		if _, ok := harnessSeatFixtures[name]; !ok {
			t.Errorf("harnessperms.go declares %s, which harnessSeatFixtures does not name — "+
				"an unregistered fixture is exempt from the parity gate by omission", name)
		}
	}
	for name := range harnessSeatFixtures {
		if !slices.Contains(declared, name) {
			t.Errorf("harnessSeatFixtures names %s, which harnessperms.go no longer declares", name)
		}
	}
}

func TestTheAdminFixtureHoldsEveryGrantANarrowerSeatFixtureDoes(t *testing.T) {
	seed := seededRoleGrants(t)
	names := make([]string, 0, len(harnessSeatFixtures))
	for name := range harnessSeatFixtures {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		role := seatRole(t, name)
		// STRICTLY narrower only. A fixture may be narrower than its own
		// seat on purpose — several here are, and say so where they are
		// declared — so this compares seats, not fixtures. ops holds the
		// admin grid and is therefore not narrower; nor is AdminWithSignals,
		// which is this fixture plus a delta.
		if !strictlyWider(seed, seatRole(t, adminFixture), role) {
			continue
		}
		for object, grant := range harnessSeatFixtures[name].Objects {
			for _, verb := range grantDiff(grant, AdminPerms.Objects[object]) {
				t.Errorf("%s (%s) holds %s.%s and %s does not, though the seed grants "+
					"admin strictly more than %s — a surface production admits to both "+
					"answers the admin as though it were refused",
					name, role, object, verb, adminFixture, role)
			}
		}
	}
}

// declaredSeatFixtures reads the package-level var names out of
// harnessperms.go. It parses rather than greps because a fixture is declared
// inside a var block, where a name is a token and not a line.
func declaredSeatFixtures(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), harnessPermsFile, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", harnessPermsFile, err)
	}
	var names []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range value.Names {
				names = append(names, name.Name)
			}
		}
	}
	sort.Strings(names)
	return names
}

// seatRole is the seeded role a fixture stands in for. A fixture naming none
// cannot be placed against the seed at all, so it fails rather than passing
// unexamined.
func seatRole(t *testing.T, fixture string) string {
	t.Helper()
	keys := harnessSeatFixtures[fixture].RoleKeys
	if len(keys) != 1 {
		t.Fatalf("%s names %d role keys, want exactly one — the gate places a fixture "+
			"against one seeded role", fixture, len(keys))
	}
	return keys[0]
}

// seededRoleGrants is the matrix identity seeds, as role → object → verb.
func seededRoleGrants(t *testing.T) map[string]map[string]principal.ObjectGrant {
	t.Helper()
	raw, err := os.ReadFile(seededDefaults)
	if err != nil {
		t.Fatalf("reading the seeded defaults: %v", err)
	}
	var documents map[string]struct {
		Objects map[string]principal.ObjectGrant `json:"objects"`
	}
	if err := json.Unmarshal(raw, &documents); err != nil {
		t.Fatalf("decoding the seeded defaults: %v", err)
	}
	out := make(map[string]map[string]principal.ObjectGrant, len(documents))
	for role, doc := range documents {
		out[role] = doc.Objects
	}
	return out
}

// strictlyWider reports whether the seed grants `wide` everything it grants
// `narrow`, and something more.
func strictlyWider(seed map[string]map[string]principal.ObjectGrant, wide, narrow string) bool {
	wideGrants, ok := seed[wide]
	if !ok {
		return false
	}
	narrowGrants, ok := seed[narrow]
	if !ok {
		return false
	}
	for object, grant := range narrowGrants {
		if len(grantDiff(grant, wideGrants[object])) > 0 {
			return false
		}
	}
	for object, grant := range wideGrants {
		if len(grantDiff(grant, narrowGrants[object])) > 0 {
			return true
		}
	}
	return false
}

// grantDiff names the verbs `held` grants that `against` does not.
func grantDiff(held, against principal.ObjectGrant) []string {
	var missing []string
	for _, verb := range []struct {
		name      string
		held, has bool
	}{
		{"create", held.Create, against.Create},
		{"read", held.Read, against.Read},
		{"update", held.Update, against.Update},
		{"delete", held.Delete, against.Delete},
	} {
		if verb.held && !verb.has {
			missing = append(missing, verb.name)
		}
	}
	return missing
}
