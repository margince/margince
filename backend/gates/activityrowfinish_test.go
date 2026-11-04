// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every function that scans activity rows also finishes them, inside the
// activities module and outside it.
//
// activities.ScanActivityRow reads the projected columns. FinishActivityRows
// adds the rest a row off /activities carries, custom-field values among it. A
// reader that fills rows by hand misses each step added to the list later.
//
// Inside the module the scan is the function ScanActivityRow wraps. A function
// that takes a pgx row and scans without finishing wraps it too, and the gate
// judges its callers in its place.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	activitiesModuleDir  = "internal/modules/activities/"
	activitiesImportPath = "github.com/margince/margince/backend/internal/modules/activities"
	exportedRowScan      = "ScanActivityRow"
	rowFinish            = "FinishActivityRows"
)

// rowScanSource is one parsed non-test source and where it sits.
type rowScanSource struct {
	where string
	file  *ast.File
}

func TestEveryActivityRowScanFinishesTheRows(t *testing.T) {
	t.Parallel()
	var inside []rowScanSource
	var findings []string
	outsideScanners := 0
	for _, path := range handWrittenGoSources(t) {
		where := filepath.ToSlash(path)
		if strings.HasSuffix(where, "_test.go") {
			continue
		}
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", where, err)
		}
		if strings.HasPrefix(where, activitiesModuleDir) {
			inside = append(inside, rowScanSource{where, file})
			continue
		}
		found, scanned := judgeActivityRowScans(where, file)
		findings = append(findings, found...)
		outsideScanners += scanned
	}
	insideFindings, insideScanners := judgeModuleRowScans(inside)
	for _, finding := range append(findings, insideFindings...) {
		t.Error(finding)
	}
	// A census that reads no scanner reports a clean pass over nothing. The
	// single-record read and the list scan inside today, the contact page outside.
	if insideScanners == 0 || outsideScanners == 0 {
		t.Fatalf("the census read %d scanner(s) inside the activities module and %d outside; "+
			"a side that reads none has gone quiet, so this gate proves nothing there", insideScanners, outsideScanners)
	}
}

// A scanner that fills its rows by hand is judged, under any import name.
func TestAnActivityRowScanWithoutTheFinishIsJudged(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(token.NewFileSet(), "internal/compose/page/timeline.go", `package page
import acts "github.com/margince/margince/backend/internal/modules/activities"
func finished(rows Rows) { acts.ScanActivityRow(rows); acts.FinishActivityRows(ctx, tx, cols, out, now) }
func byHand(rows Rows) { acts.ScanActivityRow(rows); acts.AttachLinks(ctx, tx, out) }`, 0)
	if err != nil {
		t.Fatalf("parsing the planted source: %v", err)
	}
	findings, scanned := judgeActivityRowScans("internal/compose/page/timeline.go", file)
	if scanned != 2 || len(findings) != 1 || !strings.Contains(findings[0], "byHand") {
		t.Fatalf("the census did not judge the hand-filled scanner: %d scanner(s), findings %v", scanned, findings)
	}
}

// Inside the module, a reader through a wrapper is judged, and the wrapper is not.
func TestAModuleRowScanWithoutTheFinishIsJudged(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(token.NewFileSet(), activitiesModuleDir+"read.go", `package activities
import "github.com/jackc/pgx/v5"
func ScanActivityRow(rows pgx.Rows) (Activity, error) { return scanActivity(rows) }
func scanActivity(row pgx.Row) (Activity, error) { return Activity{}, row.Scan() }
func scanPage(row pgx.Row) (Activity, error) { return scanActivity(row) }
func list(tx Tx) { scanPage(r); FinishActivityRows(ctx, tx, cols, out, now) }
func one(tx Tx) { scanActivity(r); AttachLinks(ctx, tx, out) }`, 0)
	if err != nil {
		t.Fatalf("parsing the planted source: %v", err)
	}
	findings, scanned := judgeModuleRowScans([]rowScanSource{{activitiesModuleDir + "read.go", file}})
	if scanned != 2 || len(findings) != 1 || !strings.Contains(findings[0], "one ") {
		t.Fatalf("the census did not judge the hand-filled module reader: %d scanner(s), findings %v", scanned, findings)
	}
}

// judgeModuleRowScans reports each function of the activities module that
// scans rows without finishing them, and how many functions scan. The scan set
// starts at what ScanActivityRow calls and grows by every wrapper of it.
func judgeModuleRowScans(sources []rowScanSource) (findings []string, scanners int) {
	var funcs []*ast.FuncDecl
	where := map[*ast.FuncDecl]string{}
	for _, source := range sources {
		for _, decl := range source.file.Decls {
			if fn, isFunc := decl.(*ast.FuncDecl); isFunc && fn.Body != nil {
				funcs = append(funcs, fn)
				where[fn] = source.where
			}
		}
	}
	scans := moduleRowScans(funcs)
	for _, fn := range funcs {
		calls := bareCalls(fn)
		if scans[fn.Name.Name] || fn.Name.Name == rowFinish || !callsAny(calls, scans) {
			continue
		}
		scanners++
		if !calls[rowFinish] {
			findings = append(findings, where[fn]+": "+fn.Name.Name+
				" scans activity rows and never calls FinishActivityRows; call it on the rows it returns")
		}
	}
	return findings, scanners
}

// moduleRowScans names ScanActivityRow, the function it calls, and every
// function that takes a pgx row and calls one of those without finishing.
func moduleRowScans(funcs []*ast.FuncDecl) map[string]bool {
	scans := map[string]bool{exportedRowScan: true}
	for _, fn := range funcs {
		if fn.Name.Name == exportedRowScan {
			for name := range bareCalls(fn) {
				scans[name] = true
			}
		}
	}
	for grew := true; grew; {
		grew = false
		for _, fn := range funcs {
			calls := bareCalls(fn)
			if !scans[fn.Name.Name] && takesARow(fn) && !calls[rowFinish] && callsAny(calls, scans) {
				scans[fn.Name.Name], grew = true, true
			}
		}
	}
	return scans
}

// bareCalls names every function fn calls by a bare identifier.
func bareCalls(fn *ast.FuncDecl) map[string]bool {
	calls := map[string]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if ident, ok := call.Fun.(*ast.Ident); ok {
				calls[ident.Name] = true
			}
		}
		return true
	})
	return calls
}

// takesARow reports whether fn has a pgx.Row or pgx.Rows parameter, the shape
// of a function that reads one row for its caller.
func takesARow(fn *ast.FuncDecl) bool {
	for _, field := range fn.Type.Params.List {
		sel, ok := field.Type.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "pgx" && (sel.Sel.Name == "Row" || sel.Sel.Name == "Rows") {
			return true
		}
	}
	return false
}

// judgeActivityRowScans reports each function of file that scans activity rows
// without finishing them, and how many functions scan.
func judgeActivityRowScans(where string, file *ast.File) (findings []string, scanners int) {
	name := activitiesImportName(file)
	if name == "" {
		return nil, 0
	}
	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || fn.Body == nil {
			continue
		}
		calls := map[string]bool{}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == name {
					calls[sel.Sel.Name] = true
				}
			}
			return true
		})
		if !calls[exportedRowScan] {
			continue
		}
		scanners++
		if !calls[rowFinish] {
			findings = append(findings, where+": "+fn.Name.Name+
				" scans activity rows with activities.ScanActivityRow and never calls activities.FinishActivityRows; call it on the rows it returns")
		}
	}
	return findings, scanners
}

// activitiesImportName is the name file refers to the activities module by,
// or empty when it does not import it.
func activitiesImportName(file *ast.File) string {
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil || importPath != activitiesImportPath {
			continue
		}
		if spec.Name != nil {
			return spec.Name.Name
		}
		return "activities"
	}
	return ""
}
