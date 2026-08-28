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
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"io/fs"
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

// integrationSuitesUnder parses every Go test file in the integration lane
// under these roots.
//
// Keyed on the build TAG rather than the filename: the tag is what puts a file
// in the lane, and a suite could be named anything. Comments are parsed because
// the tag is one — the only place in this gate where a comment is read on
// purpose.
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
			file, parseErr := parser.ParseFile(fset, p, nil, parser.ParseComments)
			if parseErr != nil {
				return parseErr
			}
			if !inIntegrationLane(file) {
				return nil
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

// inIntegrationLane reports whether Go would BUILD this file with the
// integration tag set.
//
// Parsed with go/build/constraint rather than matched as text, because the two
// answers differ in both directions. `//go:build !integration` contains the
// word and is the file Go EXCLUDES when the tag is on — judging it would report
// a unit-lane file for a rule it is not under. And a `//go:build` line below
// the package clause is not a constraint at all; Go ignores it, and so must
// this, or a sentence in a doc comment could enrol a file in a lane it never
// runs in.
func inIntegrationLane(file *ast.File) bool {
	for _, group := range file.Comments {
		// Constraints sit ABOVE the package clause. A group starting after it
		// is prose, whatever it says.
		if group.Pos() > file.Package {
			break
		}
		for _, line := range group.List {
			expr, err := constraint.Parse(line.Text)
			if err != nil {
				continue
			}
			// Evaluated with the tag ON, which is the question: would this file
			// be built in the integration lane.
			if expr.Eval(func(tag string) bool { return tag == "integration" }) {
				return true
			}
		}
	}
	return false
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
