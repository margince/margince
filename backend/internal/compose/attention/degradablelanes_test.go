// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// A lane the page reports as failed reads through degradable, and this is what
// holds that.
//
// The defect it comes from: every lane reads on the page's one snapshot, so a
// lane that errors there aborts the transaction under every read after it. The
// page promised "a source that failed is named, not fatal" and delivered a 500
// from whichever required read came next.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// reportsWithoutReading names a failed source from the note keepTeams returned,
// so the roster read it describes is keepTeams's own, and that one is degradable.
const reportsWithoutReading = "withRosterRefusal"

func TestEveryLaneReportedAsFailedReadsThroughDegradable(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}
	fset := token.NewFileSet()
	producers := 0
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !namesSelector(fn, "WorklistSourceUnavailableReasonFailed") {
				continue
			}
			producers++
			if fn.Name.Name != reportsWithoutReading && !namesSelector(fn, "degradable") {
				t.Errorf("%s: %s reports a failed source without reading it through degradable — "+
					"its error would abort the snapshot for every lane after it", name, fn.Name.Name)
			}
		}
	}
	if producers < 4 {
		t.Fatalf("found %d functions reporting a failed source, want the waiting, leads, plan and roster producers at least — the scan has gone blind", producers)
	}
}

func namesSelector(fn *ast.FuncDecl, name string) bool {
	found := false
	ast.Inspect(fn, func(node ast.Node) bool {
		if sel, ok := node.(*ast.SelectorExpr); ok && sel.Sel.Name == name {
			found = true
		}
		return !found
	})
	return found
}
