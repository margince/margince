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
// The scope sweeps the whole module and the count runs per function over every
// file it finds.

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
		Roots:   []string{"internal"},
		Subject: insertsInto(projectHealthTable),
		Exempt:  gatekit.Waive(map[string]string{}),
	}
	var files []*ast.File
	for _, f := range scope.Files(t) {
		files = append(files, f.File)
	}
	writers, total := projectHealthWriters(files)
	for _, fn := range writers {
		if fn != projectHealthWriter {
			t.Errorf("%s writes %s with an INSERT of its own; append through %s, which writes the "+
				"audit row and the event with it", fn, projectHealthTable, projectHealthWriter)
		}
	}
	// One, not at most one. Zero is the direction a census fails silently in:
	// the writer was renamed or moved, and "no second writer" reads the same.
	if total != 1 {
		t.Errorf("%s is written by %d INSERTs in functions, want exactly 1 in %s", projectHealthTable, total, projectHealthWriter)
	}
}

// projectHealthWriters names every function that holds an INSERT into the
// table, once per INSERT, and the total.
func projectHealthWriters(files []*ast.File) ([]string, int) {
	count := insertCounter(projectHealthTable)
	var writers []string
	total := 0
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			n := count(&ast.File{Name: ast.NewIdent("p"), Decls: []ast.Decl{fn}})
			for range n {
				writers = append(writers, fn.Name.Name)
			}
			total += n
		}
	}
	return writers, total
}

// The census names a second writer beside the real one. The fixture spells the
// table through the constant, so this file is not itself a writer the sweep finds.
func TestTheProjectHealthCensusSeesASecondWriter(t *testing.T) {
	t.Parallel()
	fixture := parseGateFixture(t, "package p\n"+
		"func "+projectHealthWriter+"() { q(`INSERT INTO "+projectHealthTable+" (id) VALUES ($1)`) }\n"+
		"func rogue() { q(`INSERT INTO "+projectHealthTable+" (id) VALUES ($1)`) }\n")
	writers, total := projectHealthWriters([]*ast.File{fixture})
	if total != 2 || len(writers) != 2 || writers[1] != "rogue" {
		t.Fatalf("the census found %v (%d), want %s and rogue", writers, total, projectHealthWriter)
	}
}
