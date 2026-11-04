// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// project_health_assessment has one INSERT, in projects' insertHealthRow.
//
// Four writers append to the history: a user's reading, a correction, the
// health job's reading and the undo of it. The audit row, the event and the
// read-back live in that one function. A fifth writer with its own INSERT would
// write a row with no event, or an AI reading nobody can tell from a user's.
// The scope proves no INSERT into the table sits outside the module. The count
// proves the one inside it is in insertHealthRow.

import (
	"go/ast"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	projectHealthTable  = "project_health_assessment"
	projectHealthWriter = "insertHealthRow"
)

func TestProjectHealthHasOneWriter(t *testing.T) {
	t.Parallel()
	scope := gatekit.Scope{
		Roots:   []string{"internal/modules/projects"},
		Subject: insertsInto(projectHealthTable),
		Exempt:  gatekit.Waive(map[string]string{}),
	}
	count := insertCounter(projectHealthTable)
	total := 0
	for _, f := range scope.Files(t) {
		for _, decl := range f.File.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			n := count(&ast.File{Name: ast.NewIdent("p"), Decls: []ast.Decl{fn}})
			total += n
			if n > 0 && fn.Name.Name != projectHealthWriter {
				t.Errorf("%s: %s writes %s with an INSERT of its own; append through %s, which writes the "+
					"audit row and the event with it", f.Path, fn.Name.Name, projectHealthTable, projectHealthWriter)
			}
		}
	}
	// One, not at most one. Zero is the direction a census fails silently in:
	// the writer was renamed or moved, and "no second writer" reads the same.
	if total != 1 {
		t.Errorf("%s is written by %d INSERTs in functions, want exactly 1 in %s", projectHealthTable, total, projectHealthWriter)
	}
}
