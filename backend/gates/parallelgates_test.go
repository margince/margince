// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// Every gate here runs in parallel with the others, and this is what keeps that
// true as gates are added.
//
// These gates are readers, and the tail is flat: a hundred independent walks of
// the same tree, no single one of them hot. That is the shape parallelism
// answers — the suite costs its slowest walk instead of the sum of all of them.
//
// The reason this needs a gate rather than a convention: a gate added without
// the call still PASSES. It just runs alone while the others share the machine,
// and the suite gets a little slower every time. Nothing fails, so nothing
// tells anybody — the cost arrives as a number in a CI log that nobody is
// comparing against last month's.
//
// WHAT THE PARALLEL GATES SHARE, since a package of concurrent tests is a fair
// thing to worry about. Both halves matter, and the second is why
// parallelExempt is not only about t.Setenv:
//
//   - Each gatekit.Waivers set. Its `matched` map is mutex-guarded, and
//     AssertAllMatched must be called from exactly one place per declaration —
//     held by TestEveryWaiversDeclarationIsSweptForStalenessExactlyOnce.
//     Getting that wrong reports staleness that is not there, which fails
//     loudly rather than passing quietly.
//   - The tree itself, which is read-only EXCEPT under the `-update-*` flags.
//     A gate holding one of those rewrites a file other gates read, and
//     os.WriteFile truncates, so a concurrent reader can see half a file.

import (
	"go/ast"
	"go/build"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// parallelExempt names the gates that must NOT take t.Parallel(), with the
// reason. Keyed by test name rather than by file: the obligation is a property
// of the function, and a file-keyed exemption would silently cover every gate
// that later joins that file.
//
// The file-rewriting entries name the WRITER, never a reader. Go resumes
// parallel tests only once every sequential one has finished, so a sequential
// writer is ordered before every parallel reader there is OR WILL BE. Exempting
// a reader instead fixes one call site and leaves the next reader somebody adds
// racing the same write.
//
// A gatekit.Waivers rather than a bare map, so the reasons are held to the same
// standard as every other waiver here and a stale entry is reported by the
// mechanism that already does that.
var parallelExempt = gatekit.Waive(map[string]string{
	// t.Setenv panics outright in a parallel test — the environment is
	// process-wide, so Go refuses the combination rather than letting one
	// test's variable leak into another's.
	"TestTheSchemaAndTheParserAgreeOnEveryInputDeclaration": "calls t.Setenv, which Go forbids in a parallel test",
	// Rewrites ../docs/reference/gate-inventory.md under -update-gate-inventory.
	// The gates that walk ../docs — link targets, rulebook direction, page
	// length — read that file.
	"TestTheGateInventoryPageIsCurrent": "rewrites the inventory page under -update-gate-inventory, which the docs gates read",
	// Rewrites danglingcitations.txt under -update-citations, which
	// TestTheCitationRegisterIsSortedAndUnique reads.
	"TestEveryCitedMigrationExists": "rewrites the citation register under -update-citations, which the register gate reads",
})

// parallelCensusFloor sits below the real number of gate tests, so a walk that
// stopped finding them fails rather than reporting that all nought are parallel.
const parallelCensusFloor = 250

func TestEveryGateRunsInParallelWithTheOthers(t *testing.T) {
	t.Parallel()
	// The sole sweep of this declaration, as gatekit requires: matched
	// accumulates across the package, so a second caller would report staleness
	// that is not there.
	//
	// Only under the default build, and that is not a skip: two of the three
	// exempt gates live in `!integration` files, so with the tag set they are
	// not in the package at all. Their entries would then match no subject —
	// which means "excluded by a build tag" and not "ratifies code that is
	// gone". The default build contains every gate, so staleness is judged
	// there, on the whole set, every run.
	if !builtWithIntegrationTag {
		defer parallelExempt.AssertAllMatched(t)
	}

	var serial, exemptButParallel []string
	seen := 0
	for _, path := range activeGateSources(t) {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		for _, decl := range parseGateFile(t, path, source).Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !isGateTest(fn) {
				continue
			}
			seen++
			named := filepath.Base(path) + ":" + fn.Name.Name
			if callsParallelFirst(fn) {
				// A gate that complies needs no exemption, and one that has
				// both has lost whatever the exemption was for. Subjects()
				// rather than Waived(): asking must not MARK the entry, or a
				// waiver stays green by being consulted while the reason it
				// ratifies is gone — which is the failure AssertAllMatched
				// exists to report.
				//
				// Both then fire, deliberately: the sweep reports it as a key
				// that matched nothing, which is true but reads as a rename,
				// and this says what actually happened.
				if slices.Contains(parallelExempt.Subjects(), fn.Name.Name) {
					exemptButParallel = append(exemptButParallel, named)
				}
				continue
			}
			if parallelExempt.Waived(t, fn.Name.Name) {
				continue
			}
			serial = append(serial, named)
		}
	}

	if seen < parallelCensusFloor {
		t.Fatalf("found %d gate test(s) and expected at least %d — this census stopped seeing the "+
			"package rather than the package having shrunk", seen, parallelCensusFloor)
	}
	for _, name := range serial {
		t.Errorf("%s does not open with t.Parallel(). These gates are readers and run "+
			"concurrently; one that does not costs the suite its own runtime on every run, and "+
			"nothing fails to say so. Add the call, or name it in parallelExempt with the reason "+
			"it cannot take one", name)
	}
	for _, name := range exemptButParallel {
		t.Errorf("%s is named in parallelExempt but opens with t.Parallel(). Something about that "+
			"gate needs it sequential and the call has taken that away, while the entry still "+
			"reads as ratifying it: remove one or the other", name)
	}
}

// activeGateSources is the gate files THIS BUILD compiles, not every file on
// disk whose name ends in _test.go.
//
// Thirteen gates carry `//go:build !integration`, so a glob and the compiler
// disagree by that many whenever the integration tag is set — and this census
// would then judge functions the build does not contain, reporting a serial
// test that Go never runs. go/build answers the question the compiler answered.
func activeGateSources(t *testing.T) []string {
	t.Helper()
	ctx := build.Default
	if builtWithIntegrationTag {
		ctx.BuildTags = append(ctx.BuildTags, "integration")
	}
	pkg, err := ctx.ImportDir(gateDir, 0)
	if err != nil {
		t.Fatalf("asking go/build which sources %s compiles: %v", gateDir, err)
	}
	paths := make([]string, 0, len(pkg.TestGoFiles))
	for _, name := range pkg.TestGoFiles {
		paths = append(paths, filepath.Join(gateDir, name))
	}
	return paths
}

// isGateTest reports whether a declaration is one of this package's gates.
//
// Shared with the inventory census rather than spelled twice: two gates
// deciding "is this a gate?" independently answer differently the day either is
// corrected.
func isGateTest(fn *ast.FuncDecl) bool {
	if fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") {
		return false
	}
	// TestMain is the package's entry point and not one of its tests — Go runs
	// it INSTEAD of the tests and hands them to it.
	return fn.Name.Name != "TestMain"
}

// callsParallelFirst reports whether the function's FIRST statement is
// t.Parallel() on its own *testing.T.
//
// First, not merely present: Go runs a parallel test's body up to the call
// synchronously, so work above it is serial work that reads as parallel. A gate
// that walked the tree and then called t.Parallel() would look annotated and
// buy nothing.
//
// The receiver is read from the signature rather than assumed to be `t`, so a
// gate is judged on what it does and not on what it named its parameter.
func callsParallelFirst(fn *ast.FuncDecl) bool {
	if fn.Body == nil || len(fn.Body.List) == 0 {
		return false
	}
	expr, ok := fn.Body.List[0].(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := expr.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Parallel" {
		return false
	}
	receiver, ok := selector.X.(*ast.Ident)
	return ok && receiver.Name == testingParamName(fn)
}

// testingParamName is what a test calls its *testing.T, or "" for a signature
// that names nothing — which matches no identifier, so such a function is
// reported rather than silently accepted.
func testingParamName(fn *ast.FuncDecl) string {
	if fn.Type.Params == nil || len(fn.Type.Params.List) == 0 {
		return ""
	}
	if names := fn.Type.Params.List[0].Names; len(names) > 0 {
		return names[0].Name
	}
	return ""
}
