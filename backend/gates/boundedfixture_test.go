// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// An integration suite that only ever acts unbounded proves nothing about row
// scope.
//
// auth.Unbounded short-circuits on RowScopeAll:
//
//	return p.Type == principal.PrincipalSystem || p.Permissions.RowScope == principal.RowScopeAll
//
// so every row-scope check downstream of it — ensureWriteAuthority,
// EnsureWritableLive, ScopeClauseFor, VisibleSubset's row arm — answers yes
// unconditionally for such a principal. A case whose fixture is unbounded can
// only test what an unbounded caller reaches, and every authority check it
// walks through is untested by it while reading as covered.
//
// This does NOT forbid an unbounded actor. An admin at RowScopeAll is the right
// fixture for a case about something else entirely, and most cases are. What it
// refuses is a SUITE PACKAGE in which no case anywhere holds a bounded
// principal, because such a package cannot have proved a single row-scope
// obligation it happens to touch.
//
// HOW IT READS A FIXTURE. Not by literal: 260 files obtain their actor through
// Env.Admin(), and several spell their permissions through a package constant,
// so a scan reading `RowScopeAll` at the call site finds nothing and agrees with
// everything. The scope is resolved from the NAME — every
// `principal.Permissions` value declared in these packages is collected first,
// with its RowScope — and a value this cannot read fails rather than being
// classified as bounded by default.
//
// An ABSENT RowScope counts as bounded, and that is not a convenience: the
// field is a string whose zero value is "", which is not RowScopeAll, so
// Unbounded does not short-circuit on it and the row clauses really do run.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const integrationSuiteRoot = "internal/compose/integration"

// unboundedScope is the one value that switches every row-scope check off.
const unboundedScope = "RowScopeAll"

// permsValuesThisCannotRead ratifies a declared `principal.Permissions` whose
// value is not a literal this scan can read. Each entry says what the value is
// and why the answer is not in doubt; anything else fails, because a fixture
// silently classified as bounded is how this census would come to agree with a
// suite it never read.
var permsValuesThisCannotRead = gatekit.Waive(map[string]string{
	"AdminWithSignals": "AdminPerms plus the warm-room signal grants " +
		"(withFullSignalGrant, harnessperms.go) — it widens the OBJECT grants of an " +
		"already-RowScopeAll admin and touches no row scope, so it is unbounded like its base",
})

// suitesWithNoBoundedActor are the packages that hold no bounded principal at
// all. Registered rather than fixed: an unbounded fixture is not a defect, and
// what to do about each of these is a question for whoever owns the suite.
//
// It only SHRINKS. A package that gains a bounded case leaves the register —
// the gate says so — and a new suite package may not join it.
var suitesWithNoBoundedActor = gatekit.Waive(map[string]string{
	"agentaccess": "the MCP transport and its authorization server: every case is about a " +
		"passport's own authority, and the row scope behind it is exercised by the tool suites " +
		"in the parent package rather than here",
	"humanonlyext": "the human-only door, which refuses an agent before any row is reached — " +
		"the refusal is the subject and a bounded seat would not change it",
	"jobfanout": "fan-out from the scheduler, whose actor is a system principal by construction; " +
		"Unbounded short-circuits on the TYPE here, so a row scope on the fixture would decide nothing",
	"webhooks": "connector ingress, where the caller is the provider and the principal is the " +
		"installation's own",
})

func TestEveryIntegrationSuiteCanProveSomethingAboutRowScope(t *testing.T) {
	t.Parallel()
	scopes, files, suites, positions := readIntegrationFixtures(t)

	// Asked about the OFFENDER — a value already found unreadable — never about
	// a candidate, so an entry decays when the fixture gains a literal as well
	// as when it disappears.
	for _, name := range sortedFixtureNames(scopes) {
		if scopes[name] != "" || permsValuesThisCannotRead.Waived(t, name) {
			continue
		}
		t.Errorf("%s declares a principal.Permissions this scan cannot read, so its row scope is "+
			"unknown — ratify it in permsValuesThisCannotRead with what the value is, or give it a "+
			"literal. Classifying it by default is how a census comes to agree with a suite it "+
			"never read", name)
	}
	permsValuesThisCannotRead.AssertAllMatched(t)

	if len(suites) == 0 {
		t.Fatalf("no integration suite packages found under %s — this gate would pass over an empty tree",
			integrationSuiteRoot)
	}

	for _, dir := range suites {
		if holdsABoundedActor(files[dir], scopes) {
			continue
		}
		name := filepath.Base(dir)
		// A suite reads as bare, so an inline permissions value whose RowScope
		// this cannot read is the one thing that could make that verdict wrong
		// — and a ratified suite would carry it past unnoticed. Reported HERE,
		// where it can change the answer, rather than everywhere: a
		// table-driven case that parameterises its scope is an ordinary fixture
		// in a suite that already holds bounded actors, and refusing those
		// would be noise in front of the finding that matters.
		for _, where := range unreadableInlineScopes(files[dir], positions) {
			t.Errorf("the %s suite reads as holding no bounded principal, and %s writes a "+
				"principal.Permissions whose RowScope this scan cannot read — so the verdict rests on "+
				"a fixture it never read. Name the scope with a principal.RowScope constant",
				name, where)
		}
		if suitesWithNoBoundedActor.Waived(t, name) {
			continue
		}
		t.Errorf("the %s suite holds no bounded principal anywhere, so every row-scope check its cases "+
			"walk through is untested by them while reading as covered. Give one case a bounded actor "+
			"(RepPerms and its siblings are the fixtures for it), or register the package in "+
			"suitesWithNoBoundedActor with the reason an unbounded actor is the whole truth there", name)
	}
	// The other direction, and the one that makes this a register rather than a
	// list: a package that gains a bounded case stops offending, stops being
	// asked, and its entry is reported here.
	suitesWithNoBoundedActor.AssertAllMatched(t)
}

// sortedFixtureNames orders the dictionary so a run reports its refusals the same way
// twice.
func sortedFixtureNames(scopes map[string]string) []string {
	names := make([]string, 0, len(scopes))
	for name := range scopes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// holdsABoundedActor reports whether the package ACTS as a bounded principal
// anywhere — inside a function body, and not as a system principal.
//
// Both qualifications are refusals to over-recognise, and over-recognition is
// the one direction this census must not fail in: it would report a suite as
// able to prove something about row scope when every case in it walks past the
// checks unconditionally.
//
//   - Inside a body, because a bounded permissions value DECLARED and never used
//     proves nothing. The declaration is a fixture nobody acts as.
//   - Not a system principal, because auth.Unbounded short-circuits on the TYPE
//     before it ever reads the row scope. A bounded scope inside a
//     `principal.Principal{Type: principal.PrincipalSystem}` is decoration: the
//     checks answer yes for that caller whatever the scope says.
func holdsABoundedActor(pkg []*ast.File, scopes map[string]string) bool {
	for _, file := range pkg {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if actsBounded(fn.Body, scopes) {
				return true
			}
		}
	}
	return false
}

// actsBounded walks one function body for a bounded actor, skipping the subtree
// of any system principal it meets.
func actsBounded(body *ast.BlockStmt, scopes map[string]string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		if literal, ok := n.(*ast.CompositeLit); ok {
			if isSystemPrincipal(literal) {
				// Not descended into: whatever scope its permissions carry,
				// Unbounded answered before reading them.
				return false
			}
			if isPermissionsLiteral(literal.Type) {
				if scope, readable := rowScopeOf(literal); readable && isBounded(scope) {
					found = true
					return false
				}
			}
		}
		if ident, ok := n.(*ast.Ident); ok {
			if scope, known := scopes[ident.Name]; known && isBounded(scope) {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// unreadableInlineScopes names every inline permissions literal whose row scope
// is not a constant, in file:line form, sorted so a run reports the same way
// twice.
func unreadableInlineScopes(pkg []*ast.File, positions *token.FileSet) []string {
	var out []string
	for _, file := range pkg {
		ast.Inspect(file, func(n ast.Node) bool {
			literal, ok := n.(*ast.CompositeLit)
			if !ok || !isPermissionsLiteral(literal.Type) {
				return true
			}
			if _, readable := rowScopeOf(literal); !readable {
				out = append(out, positions.Position(literal.Pos()).String())
			}
			return true
		})
	}
	sort.Strings(out)
	return out
}

// isSystemPrincipal reports whether a literal is a principal.Principal declaring
// the system type — the caller auth.Unbounded admits on its type alone.
func isSystemPrincipal(literal *ast.CompositeLit) bool {
	sel, ok := literal.Type.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "principal" || sel.Sel.Name != "Principal" {
		return false
	}
	for _, element := range literal.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, ok := pair.Key.(*ast.Ident); !ok || key.Name != "Type" {
			continue
		}
		value, ok := pair.Value.(*ast.SelectorExpr)
		return ok && value.Sel.Name == "PrincipalSystem"
	}
	return false
}

// isBounded reports whether a resolved scope is a BOUNDED one. An unreadable
// value ("") is not bounded here: the caller has already failed for it, and
// answering yes would let the very fixture this cannot read satisfy the census.
func isBounded(scope string) bool { return scope != "" && scope != unboundedScope }

// readIntegrationFixtures parses the integration tree once and returns every
// declared permissions value's row scope, the files of each package, and the
// packages that actually hold a suite.
func readIntegrationFixtures(
	t *testing.T,
) (map[string]string, map[string][]*ast.File, []string, *token.FileSet) {
	t.Helper()
	scopes := map[string]string{}
	files := map[string][]*ast.File{}
	suiteSet := map[string]bool{}
	fset := token.NewFileSet()

	err := filepath.Walk(integrationSuiteRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		parsed, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return perr
		}
		dir := filepath.Dir(path)
		files[dir] = append(files[dir], parsed)
		// A suite is a directory that holds at least one integration case. The
		// helper packages beside them (apptest, jobtest) declare fixtures and
		// assert nothing, so a register entry for one would name a package with
		// no cases to improve.
		if strings.HasSuffix(path, "_integration_test.go") {
			suiteSet[dir] = true
		}
		collectPermissionsValues(t, parsed, scopes)
		return nil
	})
	if err != nil {
		t.Fatalf("reading %s: %v", integrationSuiteRoot, err)
	}
	// A value DERIVED from a permissions value is one too. `AdminWithSignals =
	// withFullSignalGrant(AdminPerms)` is the case: no literal to read and no
	// declared type, so the only evidence it is permissions at all is the
	// argument it was built from — which is why this is resolved from the
	// arguments rather than from the function's name. Recorded unreadable, so
	// the caller refuses it by name unless it is ratified.
	for _, pkg := range files {
		for _, file := range pkg {
			collectDerivedPermissionsValues(file, scopes)
		}
	}
	if len(scopes) == 0 {
		t.Fatalf("no principal.Permissions fixtures found under %s — this gate would classify every "+
			"package as bare", integrationSuiteRoot)
	}

	suites := make([]string, 0, len(suiteSet))
	for dir := range suiteSet {
		suites = append(suites, dir)
	}
	sort.Strings(suites)
	return scopes, files, suites, fset
}

// collectPermissionsValues records every package-level `X = principal.Permissions{…}`
// with its row scope. A value that is not a literal is recorded as unreadable
// ("") rather than skipped, so the caller can refuse it by name.
func collectPermissionsValues(t *testing.T, file *ast.File, into map[string]string) {
	t.Helper()
	// The dictionary is keyed by NAME, which two packages could both declare.
	// Whichever won would answer for the other's suites, so a name carrying two
	// different scopes fails rather than resolving to one of them. There is one
	// such name today and both declarations agree; a disagreement is the drift
	// this refuses.
	record := func(name, scope string) {
		if was, seen := into[name]; seen && was != scope {
			t.Errorf("%s is declared as a principal.Permissions in more than one integration package "+
				"with different row scopes (%q and %q), so a suite referring to it would be classified "+
				"by whichever declaration this read last — qualify the names, or give them distinct ones",
				name, was, scope)
		}
		into[name] = scope
	}

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
				if i >= len(value.Values) {
					continue
				}
				if literal, ok := value.Values[i].(*ast.CompositeLit); ok {
					if isPermissionsLiteral(literal.Type) {
						scope, readable := rowScopeOf(literal)
						if !readable {
							scope = ""
						}
						record(name.Name, scope)
					}
					continue
				}
				if isPermissionsLiteral(value.Type) {
					record(name.Name, "")
				}
			}
		}
	}
}

// collectDerivedPermissionsValues records a package-level var whose value is a
// call carrying a known permissions value. It runs after every literal is
// known, so the argument it keys on has already been classified.
func collectDerivedPermissionsValues(file *ast.File, into map[string]string) {
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
				if i >= len(value.Values) {
					continue
				}
				if _, already := into[name.Name]; already {
					continue
				}
				call, ok := value.Values[i].(*ast.CallExpr)
				if !ok {
					continue
				}
				for _, arg := range call.Args {
					ident, ok := arg.(*ast.Ident)
					if !ok {
						continue
					}
					if _, known := into[ident.Name]; known {
						into[name.Name] = ""
						break
					}
				}
			}
		}
	}
}

func isPermissionsLiteral(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "principal" && sel.Sel.Name == "Permissions"
}

// rowScopeOf reads the RowScope field's constant name, and says whether it
// could read it at all.
//
// An absent field is the zero value "", which Unbounded does not short-circuit
// on — so it is READABLE, and reported as a bounded scope under its own name.
// A value that is not a constant selector is not: a computed scope could be
// anything, and answering "not RowScopeAll" for it would let the one fixture
// this cannot read satisfy the census.
func rowScopeOf(literal *ast.CompositeLit) (scope string, readable bool) {
	for _, element := range literal.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, ok := pair.Key.(*ast.Ident); !ok || key.Name != "RowScope" {
			continue
		}
		if sel, ok := pair.Value.(*ast.SelectorExpr); ok {
			return sel.Sel.Name, true
		}
		return "", false
	}
	return "RowScopeUnset", true
}
