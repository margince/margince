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
	"strconv"
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
	pkg := principalIdent(t, file)
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
			for i, name := range value.Names {
				// The blank identifier names nothing a suite can act
				// through, so it is not a fixture whatever it is assigned.
				if name.Name == "_" || !mayBeASeatFixture(value, i, pkg) {
					continue
				}
				names = append(names, name.Name)
			}
		}
	}
	sort.Strings(names)
	return names
}

// mayBeASeatFixture reports whether one NAME in a var declaration could be a
// seat fixture, which is not the same question as whether it IS one.
//
// It answers by EXCLUSION — a composite literal of some other type is not a
// fixture, so a shared ObjectGrant parked in this file does not have to be
// registered — and everything else has to be. That asymmetry is the point.
// AdminWithSignals is `withFullSignalGrant(AdminPerms)`, a call and not a
// literal, and admitting only literals would have exempted the one fixture in
// the file that is assembled rather than written out. Deciding this properly
// needs the type checker, and the package does not depend on one; excluding
// what is provably not a fixture is what can be done without it, and every
// judgement below errs toward demanding registration.
//
// PER NAME, not per declaration: `var A, B = principal.Permissions{…},
// principal.ObjectGrant{…}` declares one fixture and one thing that is not,
// and excluding the whole spec would drop A silently — an exemption by
// omission, which is what this census exists to refuse.
func mayBeASeatFixture(spec *ast.ValueSpec, index int, principalPkg string) bool {
	if spec.Type != nil {
		return isPermissionsType(spec.Type, principalPkg)
	}
	// The multi-return form (`var a, b = f()`) has no per-name value to
	// inspect, so it is asked for.
	if len(spec.Values) != len(spec.Names) {
		return true
	}
	literal, ok := spec.Values[index].(*ast.CompositeLit)
	return !ok || isPermissionsType(literal.Type, principalPkg)
}

func isPermissionsType(expr ast.Expr, principalPkg string) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	return ok && pkg.Name == principalPkg && selector.Sel.Name == "Permissions"
}

// principalPackage is the import path of the package whose Permissions type
// every fixture here has.
const principalPackage = "github.com/margince/margince/backend/internal/shared/kernel/principal"

// principalIdent is the identifier harnessperms.go qualifies that package by.
//
// Resolved rather than assumed. Matching the bare name `principal` would read
// an aliased import as some other package, and every Permissions literal in
// the file would then be excluded — silently, with no test failing, which is
// exactly the escape this census exists to close. A file that does not import
// the package at all, or that dot-imports it so no qualifier survives to
// match, is not a file this gate can reason about, and it says so rather than
// passing on an empty census.
func principalIdent(t *testing.T, file *ast.File) string {
	t.Helper()
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || path != principalPackage {
			continue
		}
		if spec.Name == nil {
			return "principal"
		}
		if name := spec.Name.Name; name != "." && name != "_" {
			return name
		}
		t.Fatalf("%s imports %s as %q, which leaves no qualifier to match — this gate "+
			"reads the fixtures' type off the source", harnessPermsFile, principalPackage, spec.Name.Name)
	}
	t.Fatalf("%s does not import %s, whose Permissions type every fixture in it has — "+
		"an empty census would pass while governing nothing", harnessPermsFile, principalPackage)
	return ""
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
	// The same question principal.ObjectGrant.Contains answers, kept separate
	// because this one has to NAME the missing verbs: a parity failure that says
	// "narrower" sends the reader back to two maps to work out which verb, and
	// the whole value of this gate is landing on the line to change.
	//
	// Held by: TestGrantDiffAgreesWithContains below, so the two cannot drift —
	// a gate reporting no difference while the guard refuses, or the reverse, is
	// the failure that would otherwise be invisible in both directions.
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

// The admin fixture holds every grant the seeded admin role holds.
//
// The arm above compares fixtures to EACH OTHER, so it answers "is the widest
// stand-in narrower than something it stands in for". It cannot answer the
// question that actually bit: is the widest stand-in narrower than the SEAT
// itself. Removing a grant from AdminPerms leaves it passing, because every
// other fixture is narrower still.
//
// That gap is how thirteen objects went missing at once. When a settings surface
// moves off a literal `admin` role check onto a grant, every integration test
// driving that surface as an admin starts failing — and the 403 it produces is
// indistinguishable from a correctly refused request, so the repair looks like
// "the test was wrong" rather than "the fixture is short".
//
// Compared object by object rather than as a whole, because the fixture is
// allowed to be narrower in one deliberate direction: it omits objects no
// integration test drives. What it may never do is hold a NARROWER grant on an
// object it does name — that is a fixture claiming to be an admin while
// answering like somebody else.
func TestTheAdminFixtureIsNotNarrowerThanTheSeededAdminRole(t *testing.T) {
	seeded := seededRoleGrants(t)[seatRole(t, adminFixture)]
	if len(seeded) == 0 {
		t.Fatal("the seeded admin document decoded to no objects — this gate is comparing " +
			"against nothing and would pass whatever the fixture held")
	}
	// Iterating the SEED and not the fixture. Walking the fixture visits only
	// objects it already names, so an object deleted from it is never looked at
	// — the shape that let this gate pass its own first mutation, and the same
	// under-recognition the whole file is written against.
	for object, granted := range seeded {
		if _, named := AdminPerms.Objects[object]; !named {
			// Deliberately narrower: the fixture omits objects no integration
			// test drives, and adding all of them would make several suites pass
			// for the wrong reason. Absence is a decision; a NARROWER grant on an
			// object it does name is not.
			continue
		}
		for _, verb := range grantDiff(granted, AdminPerms.Objects[object]) {
			t.Errorf("the seeded admin role holds %s.%s and %s does not — every integration "+
				"test driving that surface as an admin gets a 403 that reads exactly like a "+
				"correct refusal, so the fixture is repaired by widening it here, never by "+
				"weakening the test", object, verb, adminFixture)
		}
	}
}

// The ops fixture matches the seeded ops role on every governance object.
//
// OpsPerms is the admin grid minus what ops does not hold, and that subtraction
// is hand-maintained. It was correct while the difference was a role NAME — the
// fixture could take the admin map whole and the literal-admin gates did the
// separating. Now the difference lives in grants, so the subtraction is load
// bearing: one object too few and a suite proves ops is refused where production
// admits it; one too many and a refusal test passes against a fixture that was
// never going to be admitted anyway.
//
// Only the governance objects are compared. Everywhere else ops genuinely holds
// the admin grid, and the arm above already refuses a fixture narrower than the
// seat it stands in for.
func TestTheOpsFixtureMatchesTheSeededOpsRoleOnGovernance(t *testing.T) {
	seed := seededRoleGrants(t)
	ops := seed[seatRole(t, "OpsPerms")]
	if len(ops) == 0 {
		t.Fatal("the seeded ops document decoded to no objects — this gate is comparing " +
			"against nothing")
	}
	// Derived from the seed rather than listed: an object added later that ops
	// holds differently from admin is inside this census without anybody
	// remembering to add it, which is the way a hand-listed corpus fails short.
	admin := seed[seatRole(t, adminFixture)]
	for object, adminGrant := range admin {
		opsGrant := ops[object]
		if opsGrant == adminGrant {
			continue
		}
		if fixture, named := OpsPerms.Objects[object]; named {
			if fixture != opsGrant {
				t.Errorf("the seed grants ops %+v on %s; the fixture carries %+v — a suite "+
					"driving that surface as ops reaches a different answer than production",
					opsGrant, object, fixture)
			}
			continue
		}
		if opsGrant != (principal.ObjectGrant{}) {
			t.Errorf("the seed grants ops %+v on %s and the fixture names it nowhere — "+
				"a refusal test on that surface passes without ever testing the gate",
				opsGrant, object)
		}
	}
}

// grantDiff and principal.ObjectGrant.Contains answer the same question.
//
// One returns which verbs are missing and the other whether any are, so they are
// two functions on purpose. What must not happen is them disagreeing: this gate
// would then pass a fixture the escalation guards refuse, or fail one they
// admit, and either way the number it reports would be about itself.
//
// Exhaustive over all 256 pairs, because the defect is per-verb and a sampled
// table would miss exactly the verb somebody forgot.
func TestGrantDiffAgreesWithContains(t *testing.T) {
	grants := make([]principal.ObjectGrant, 0, 16)
	for bits := range 16 {
		grants = append(grants, principal.ObjectGrant{
			Create: bits&1 != 0, Read: bits&2 != 0,
			Update: bits&4 != 0, Delete: bits&8 != 0,
		})
	}
	for _, held := range grants {
		for _, against := range grants {
			contained := against.Contains(held)
			if missing := grantDiff(held, against); contained != (len(missing) == 0) {
				t.Errorf("held=%+v against=%+v: Contains says %v, grantDiff reports %v — "+
					"the parity gate and the escalation guards would reach opposite answers "+
					"about the same pair", held, against, contained, missing)
			}
		}
	}
}
