// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// Every integration test reaches Postgres through a database of its own
// (testdb.New and testdb.For), and runs in parallel with its siblings.
//
// A test that reads the lane's package DSN shares one database with every
// other test of its package, so its rows reach theirs. And a test with a
// database of its own that does not call t.Parallel spends the time the copy
// costs without winning it back.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// testDSNVariables are the lane's package DSNs. Only testdb reads them.
var testDSNVariables = map[string]bool{"MARGINCE_TEST_DSN": true, "MARGINCE_TEST_APP_DSN": true}

// directDSNReaders may read a package DSN themselves, each for a reason a
// database of the test's own cannot serve.
var directDSNReaders = gatekit.Waive(map[string]string{
	"cmd/migrate/main_integration_test.go":                         "the db verbs create and drop databases on the cluster, so the test derives their names and the maintenance database from the package DSN",
	"tools/extmigrategate/throwawaydb_integration_test.go":         "the gate makes a vanilla throwaway database of its own on the cluster and migrates it through EnsureSchema",
	"internal/platform/database/readadmission_integration_test.go": "package database cannot import testdb, which imports it; the test only runs SELECT 1",
})

// serialDirective marks a test that must run alone, followed by the reason.
// It sits in the test's doc comment, next to what makes the test serial.
const serialDirective = "//testdb:serial "

// integrationLaneFiles parses every Go file the integration lane compiles.
func integrationLaneFiles(t *testing.T) map[string]*ast.File {
	t.Helper()
	files := map[string]*ast.File{}
	err := gatekit.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		if !isIntegrationTagged(path) {
			return nil
		}
		file, err := gatekit.ParseFile(path, parser.ParseComments)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(path)] = file
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module for integration-lane files: %v", err)
	}
	return files
}

func readsTestDSN(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return !found
		}
		if value, err := strconv.Unquote(lit.Value); err == nil && testDSNVariables[value] {
			found = true
		}
		return !found
	})
	return found
}

func TestOnlyTestdbReadsTheLanesPackageDSN(t *testing.T) {
	t.Parallel()
	var offenders []string
	testdbReads := false
	for path, file := range integrationLaneFiles(t) {
		if !readsTestDSN(file) {
			continue
		}
		if strings.HasPrefix(path, testdbPackage+"/") {
			testdbReads = true
			continue
		}
		if !directDSNReaders.Waived(t, path) {
			offenders = append(offenders, path)
		}
	}
	// The floor: testdb itself reads them, so a walk that finds no reader there
	// read the wrong tree or misses the spelling.
	if !testdbReads {
		t.Fatalf("no file under %s reads MARGINCE_TEST_DSN, so this gate's walk reads nothing it should", testdbPackage)
	}
	if len(offenders) > 0 {
		t.Errorf("%d integration-lane file(s) read MARGINCE_TEST_DSN or MARGINCE_TEST_APP_DSN themselves and so share the package's database. Use testdb.New(t) in a fixture and testdb.For(t) in a helper:\n\t%s",
			len(offenders), strings.Join(slices.Sorted(slices.Values(offenders)), "\n\t"))
	}
	directDSNReaders.AssertAllMatched(t)
}

func TestEveryIntegrationTestRunsInParallel(t *testing.T) {
	t.Parallel()
	var offenders []string
	tests := 0
	for path, file := range integrationLaneFiles(t) {
		if !strings.HasSuffix(path, "_test.go") {
			continue
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") || fn.Name.Name == "TestMain" || fn.Body == nil {
				continue
			}
			tests++
			serial := serialReason(fn.Doc)
			switch {
			case callsParallel(fn.Body) && serial != "":
				offenders = append(offenders, path+"#"+fn.Name.Name+" (both parallel and marked serial)")
			case !callsParallel(fn.Body) && serial == "":
				offenders = append(offenders, path+"#"+fn.Name.Name)
			}
		}
	}
	if tests == 0 {
		t.Fatal("found no integration test at all, so this gate's walk reads nothing it should")
	}
	if len(offenders) > 0 {
		t.Errorf("%d integration test(s) neither call t.Parallel() nor carry %q with a reason. Each has a database of its own, so it can run beside its siblings:\n\t%s",
			len(offenders), serialDirective+"<reason>", strings.Join(slices.Sorted(slices.Values(offenders)), "\n\t"))
	}
}

// serialReason is the reason a test's doc comment gives for running alone.
func serialReason(doc *ast.CommentGroup) string {
	if doc == nil {
		return ""
	}
	for _, c := range doc.List {
		if reason, ok := strings.CutPrefix(c.Text, serialDirective); ok {
			return strings.TrimSpace(reason)
		}
	}
	return ""
}

// callsParallel reports whether the test body itself calls t.Parallel.
func callsParallel(body *ast.BlockStmt) bool {
	for _, stmt := range body.List {
		call, ok := stmt.(*ast.ExprStmt)
		if !ok {
			continue
		}
		if c, ok := call.X.(*ast.CallExpr); ok {
			if sel, ok := c.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Parallel" {
				return true
			}
		}
	}
	return false
}
