// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

package backendarch

// What the concurrency-guard census judges a function on, driven with SYNTHETIC
// source rather than the tree — the same reason retainedcolumncases_test.go
// gives for its own cases. That census is supposed to pass, so a reader proven
// only by "the tree is clean" is one that keeps passing after it stops working:
// a statement it stops seeing produces no finding, only a smaller silence.
//
// The MISSED rows below are shapes the reader could not see before the
// package-level folding was added. Every one of them was green while an
// unguarded by-id UPDATE ran, because the statement was held one identifier
// away from the function that sent it. Rule 8 asks what shape of the defect a
// gate cannot see and says to plant that case; this is that list.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestTheGuardCensusJudgesEveryStatementAFunctionAnswersFor(t *testing.T) {
	const marker = `UPDATE organization SET legal_name = $2 WHERE id = $1`

	for _, tc := range []struct {
		name   string
		source string
		judged bool
	}{
		{
			name: "written in the body, which is the shape the census always saw",
			source: `package p
func write(tx T) { tx.Exec(ctx, ` + "`" + marker + "`" + `) }`,
			judged: true,
		}, {
			// The shape the tree already wrote: coldStartColumns and
			// companyFields held eight organization writes between them, and
			// the function that sent each one named a table rather than a
			// statement.
			name: "held in a package-level table the function indexes",
			source: `package p
var held = map[string]string{"legal_name": ` + "`" + marker + "`" + `}
func write(tx T, column string) { tx.Exec(ctx, held[column]) }`,
			judged: true,
		}, {
			name: "held in a package-level table of structs",
			source: `package p
type spec struct{ update string }
var held = []spec{{update: ` + "`" + marker + "`" + `}}
func write(tx T) { tx.Exec(ctx, held[0].update) }`,
			judged: true,
		}, {
			name: "held in a package-level const the function names",
			source: `package p
const held = ` + "`" + marker + "`" + `
func write(tx T) { tx.Exec(ctx, held) }`,
			judged: true,
		}, {
			// Neither half carries the statement: the first has no WHERE, the
			// second no SET. A reader that kept only the parts saw two
			// fragments and matched neither.
			name: "assembled at package level from two literals",
			source: `package p
var held = ` + "`UPDATE organization SET legal_name = $2 `" + ` + ` + "`WHERE id = $1`" + `
func write(tx T) { tx.Exec(ctx, held) }`,
			judged: true,
		}, {
			// The direction that MISSES a writer is the dangerous one, but this
			// is the direction that invents one: a local of the same name is
			// not the package's value, and crediting it would attribute a
			// statement to a function that never sends it.
			name: "a local shadowing the package name",
			source: `package p
var held = ` + "`" + marker + "`" + `
func write(tx T) { held := "SELECT 1"; tx.Exec(ctx, held) }`,
			judged: false,
		}, {
			// A whole-function shadow set answers this one wrong, and wrong in
			// the silent direction: the send is the package's statement, and a
			// local declared in a branch that does not contain it says nothing
			// about it. Go scopes the inner name to the inner block, and so
			// does the reader.
			name: "a local shadowing inside a branch the send is not in",
			source: `package p
var held = ` + "`" + marker + "`" + `
func write(tx T, cond bool) {
	tx.Exec(ctx, held)
	if cond {
		held := "SELECT 1"
		tx.Exec(ctx, held)
	}
}`,
			judged: true,
		}, {
			name: "a send inside the branch that shadows, and nowhere else",
			source: `package p
var held = ` + "`" + marker + "`" + `
func write(tx T, cond bool) {
	if cond {
		held := "SELECT 1"
		tx.Exec(ctx, held)
	}
}`,
			judged: false,
		}, {
			// The right-hand side stands before the declaration it feeds, so
			// this reads the package's statement and then sends it under a
			// local name.
			name: "a local initialised from the package value it shadows",
			source: `package p
var held = ` + "`" + marker + "`" + `
func write(tx T) { held := held; tx.Exec(ctx, held) }`,
			judged: true,
		}, {
			name: "a parameter shadowing the package name",
			source: `package p
var held = ` + "`" + marker + "`" + `
func write(tx T, held string) { tx.Exec(ctx, held) }`,
			judged: false,
		}, {
			name: "a package-level name this function never mentions",
			source: `package p
var held = ` + "`" + marker + "`" + `
func write(tx T) { tx.Exec(ctx, "SELECT 1") }`,
			judged: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "synthetic.go", tc.source, 0)
			if err != nil {
				t.Fatalf("parsing the case source: %v", err)
			}
			held := packageLevelStatements([]*ast.File{file})
			var fn *ast.FuncDecl
			for _, decl := range file.Decls {
				if candidate, isFunc := decl.(*ast.FuncDecl); isFunc && candidate.Name.Name == "write" {
					fn = candidate
				}
			}
			if fn == nil {
				t.Fatal("the case source declares no write function, so it proves nothing")
			}

			seen := false
			for _, statement := range statementsJudged(fn, held) {
				if strings.Contains(statement, "SET legal_name") && strings.Contains(statement, "WHERE id =") {
					seen = true
				}
			}
			if seen != tc.judged {
				t.Errorf("statementsJudged saw the write = %t, want %t — a statement the census does not "+
					"judge is one it reports PASS over rather than reporting a gap in", seen, tc.judged)
			}
		})
	}
}
