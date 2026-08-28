// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// Every pool the integration lane opens is inside the lane's budget.
//
// The lane declares a connection budget (scripts/lib-testdb.sh) and hands each
// process a per-pool ceiling in MARGINCE_TEST_POOL_MAX_CONNS. testdb applies
// it. Nothing else did: a suite reaching for database.NewPool got that
// constructor's fallback instead — MaxConns 16, MinConns 2 dialled eagerly — so
// one package could hold 8 (owner) + 8 (app) + 16 (its own) against a declared
// allowance of 24 while the lane's own arithmetic read green.
//
// The budget was therefore a MEASURED high-water mark and not a ceiling, and
// the term in lib-testdb.sh said so. This is what makes it the second thing:
// the ceiling applies by construction, because testdb.Pool and testdb.OwnPool
// are the only ways in.
//
// It reads the SYNTAX rather than the file's text, which is not a style
// preference: this repository has already shipped a lane gate that grepped
// prose and reported a test for explaining itself, and two suites here name
// database.NewPool in comments for good reasons — one describing what the
// constructor honours, one describing what its pool is built through. A gate
// that made those authors reword true sentences would teach the wrong lesson
// about a rule they keep.

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// The two constructors that open a pool without the lane's ceiling, by the
// import path each belongs to — never by the bare name, so a local helper
// called NewPool is not mistaken for the product's.
var unboundedPoolDoors = map[string][]string{
	"github.com/margince/margince/backend/internal/platform/database": {"NewPool"},
	"github.com/jackc/pgx/v5/pgxpool":                                 {"New", "NewWithConfig"},
}

// laneBudgetExempt ratifies the files that may open a pool the ceiling does not
// bound. Each is a file whose SUBJECT is the pool itself.
var laneBudgetExempt = gatekit.Waive(map[string]string{
	"internal/platform/testdb/pool_integration_test.go":           "the ceiling's own test: it asserts what testdb.Pool does to a DSN, which it can only do by opening one the other way and comparing",
	"internal/platform/testdb/laneconnbudget_integration_test.go": "the lane arithmetic's own test — it opens pools to count them, which is the measurement",
	"internal/platform/testdb/quiesce_integration_test.go":        "the quiesce probe's own test: it needs a pool it can leave busy, which the shared one must never be",
})

// TestNoIntegrationSuiteOpensAnUnboundedPool fails on a suite that opens a pool
// outside the lane's ceiling.
func TestNoIntegrationSuiteOpensAnUnboundedPool(t *testing.T) {
	t.Parallel()
	defer laneBudgetExempt.AssertAllMatched(t)

	// Walked directly rather than through gatekit.Scope, which sweeps PRODUCT
	// sources: it excludes _test.go by construction, and this gate's whole
	// subject is test files. The negative-space proof Scope buys is bought here
	// by the floor below instead.
	files := integrationSuitesUnder(t, "internal", "cmd")

	// A prohibition over an empty corpus prohibits nothing, and this one's
	// corpus is a build-tagged subset that a changed tag spelling would empty
	// without changing a single test.
	if len(files) < 100 {
		t.Fatalf("found %d integration suite(s) under internal/ and cmd/, and the lane has more than that: "+
			"this gate is no longer recognising the files it judges", len(files))
	}

	var offences []string
	for _, src := range files {
		if laneBudgetExempt.Waived(t, src.Path) {
			continue
		}
		offences = append(offences, unboundedPoolsIn(src)...)
	}
	sort.Strings(offences)
	for _, offence := range offences {
		t.Errorf("%s — the lane's budget is sized on the ceiling testdb hands out, and a pool that ignores "+
			"it makes that budget fiction. Open it with testdb.Pool (shared, memoized) or testdb.OwnPool "+
			"(your own, still bounded), or ratify the file in laneBudgetExempt with the reason its subject "+
			"IS the pool", offence)
	}
}

// integrationSuitesUnder parses every Go test file the integration lane would
// build under these roots.
//
// The lane decision is go/build's, not this file's. A hand-rolled reading of
// the constraint gets two things wrong in the direction a prohibition must
// never be wrong — it EXCLUDES files, so their pool constructors go unjudged
// and the gate reads green:
//
//   - `//go:build integration && linux` is false unless linux is set too, and
//     the lane is linux. Evaluating with the integration tag alone drops every
//     platform-qualified suite.
//   - two legacy `// +build` lines are combined by Go with AND. Reading them
//     one at a time and taking the first that passes is OR, which admits files
//     Go would not build and, worse, is a different rule from the one the
//     compiler applies.
//
// MatchFile also applies the `_linux.go` / `_test.go` filename rules, which is
// a third thing worth not reimplementing.
func integrationSuitesUnder(t *testing.T, roots ...string) []gatekit.ParsedFile {
	t.Helper()
	fset := token.NewFileSet()
	var suites []gatekit.ParsedFile
	for _, root := range roots {
		err := filepath.WalkDir(root, func(p string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(p, "_test.go") {
				return nil
			}
			built, err := laneWouldBuild(filepath.Dir(p), entry.Name())
			if err != nil {
				return err
			}
			if !built {
				return nil
			}
			file, parseErr := parser.ParseFile(fset, p, nil, parser.ParseComments)
			if parseErr != nil {
				return parseErr
			}
			suites = append(suites, gatekit.ParsedFile{Path: filepath.ToSlash(p), File: file})
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s for integration suites: %v", root, err)
		}
	}
	return suites
}

// laneContexts are the build contexts the integration lane runs in.
//
// TWO, and a file matching EITHER is judged. The lane runs on linux with cgo,
// which is the authoritative one — a suite this gate must judge is one CI would
// build. The local context is asked as well so a developer running the gate on
// another platform still judges the platform-qualified suites their own machine
// builds, rather than getting a quieter answer than CI's from the same command.
//
// Judging MORE files is the safe direction for a prohibition: the cost of an
// extra file is a finding somebody reads, and the cost of a missing one is the
// gate reading green over the thing it exists to refuse.
func laneContexts() []build.Context {
	lane := build.Default
	lane.GOOS, lane.GOARCH, lane.CgoEnabled = "linux", "amd64", true
	lane.BuildTags = append([]string{"integration"}, build.Default.BuildTags...)

	local := build.Default
	local.CgoEnabled = true
	local.BuildTags = append([]string{"integration"}, build.Default.BuildTags...)
	return []build.Context{lane, local}
}

// laneWouldBuild reports whether the integration lane compiles this file.
func laneWouldBuild(dir, name string) (bool, error) {
	for _, ctxt := range laneContexts() {
		match, err := ctxt.MatchFile(dir, name)
		if err != nil {
			return false, fmt.Errorf("reading %s's build constraint: %w", filepath.Join(dir, name), err)
		}
		if match {
			return true, nil
		}
	}
	return false, nil
}

// unboundedPoolsIn names each call in one suite that opens a pool the lane's
// ceiling does not reach.
func unboundedPoolsIn(src gatekit.ParsedFile) []string {
	var offences []string
	for importPath, names := range unboundedPoolDoors {
		qualifier, dotImported := gatekit.ImportedAs(src.File, importPath)
		if dotImported {
			offences = append(offences, src.Path+" dot-imports "+path.Base(importPath)+
				", and this gate cannot then tell its constructor from a local function of the same name")
			continue
		}
		if qualifier == "" {
			continue
		}
		for _, name := range names {
			if callsQualified(src.File, qualifier, name) {
				offences = append(offences, src.Path+" opens a pool with "+qualifier+"."+name)
			}
		}
	}
	return offences
}

// callsQualified reports a call to pkg.name — the CALL, not a mention. Comments
// are not part of the syntax tree, so a suite that names the constructor while
// explaining something is not judged for it.
func callsQualified(file *ast.File, pkg, name string) bool {
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return !found
		}
		selector, isSelector := call.Fun.(*ast.SelectorExpr)
		if !isSelector || selector.Sel.Name != name {
			return !found
		}
		if base, isIdent := selector.X.(*ast.Ident); isIdent && base.Name == pkg {
			found = true
		}
		return !found
	})
	return found
}

// TestTheLaneCensusReadsBuildConstraintsTheWayGoDoes holds the census's
// membership rule against go/build's.
//
// Membership is where a prohibition like this fails quietly: a file the census
// does not judge is not reported, and a gate that judged fewer files than it
// claims reads exactly like a clean tree. Every case below was wrong in an
// earlier version that evaluated the constraint by hand — `integration &&
// linux` came out false because only the integration tag was set, and two
// legacy lines were read as OR where Go combines them with AND.
//
// Synthetic files, because the shapes that matter are ones this tree does not
// happen to contain, and a census over what it happens to contain proves
// nothing about the next file somebody writes.
func TestTheLaneCensusReadsBuildConstraintsTheWayGoDoes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, probe := range []struct {
		what       string
		constraint string
		built      bool
	}{
		{"the plain lane tag", "//go:build integration\n", true},
		// The lane runs on linux, so a suite qualified to it IS one this gate
		// must judge. Evaluating with the integration tag alone made this false
		// and dropped every platform-qualified suite.
		{"qualified to the lane's platform", "//go:build integration && linux\n", true},
		{"qualified to cgo, which the lane enables", "//go:build integration && cgo\n", true},
		// The file Go EXCLUDES when the tag is on. It contains the word, which
		// is why a text match reported it.
		{"negated", "//go:build !integration\n", false},
		// Go combines separate legacy lines with AND. Reading them one at a
		// time and taking the first that passes is OR.
		{"two legacy lines, both satisfied", "// +build integration\n// +build linux\n", true},
		{"two legacy lines, the second unsatisfiable", "// +build integration\n// +build ignoreme\n", false},
		// An unconstrained file is built under EVERY tag set, the lane's
		// included, so it is judged here — and that is the right answer twice
		// over: it does run in the lane, and it also runs in the unit lane,
		// where check-test-lanes.sh forbids it a real connection at all. A file
		// that cannot legitimately open a pool costs this census nothing.
		{"no constraint at all — built in every lane", "", true},
	} {
		t.Run(probe.what, func(t *testing.T) {
			name := "probe_test.go"
			source := probe.constraint + "\npackage probe\n"
			if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			built, err := laneWouldBuild(dir, name)
			if err != nil {
				t.Fatalf("reading the constraint: %v", err)
			}
			if built != probe.built {
				t.Errorf("the lane builds it = %t, want %t — a file this census does not judge is a file "+
					"whose pool constructors go unreported, and the gate then reads like a clean tree",
					built, probe.built)
			}
		})
	}
}
