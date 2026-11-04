// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every function outside the activities module that scans activity rows also
// finishes them.
//
// activities.ScanActivityRow reads the projected columns. FinishActivityRows
// adds the rest a row off /activities carries, custom-field values among it. A
// reader that fills rows by hand misses each step added to the list later.

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
)

func TestEveryActivityRowScanOutsideTheModuleFinishesTheRows(t *testing.T) {
	t.Parallel()
	scanners := 0
	for _, path := range handWrittenGoSources(t) {
		where := filepath.ToSlash(path)
		if strings.HasSuffix(where, "_test.go") || strings.HasPrefix(where, activitiesModuleDir) {
			continue
		}
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", where, err)
		}
		findings, scanned := judgeActivityRowScans(where, file)
		scanners += scanned
		for _, finding := range findings {
			t.Error(finding)
		}
	}
	// A census that reads no scanner reports a clean pass over nothing. The
	// contact page's timeline is one today.
	if scanners == 0 {
		t.Fatal("no function outside the activities module calls activities.ScanActivityRow; the reader has gone quiet, so this gate proves nothing")
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
		if !calls["ScanActivityRow"] {
			continue
		}
		scanners++
		if !calls["FinishActivityRows"] {
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
