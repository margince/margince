// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

package gates

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

// marker is the write every case either sends or does not. Shared by all of
// them so that a case differs from its neighbours in WHERE the statement lives
// and in nothing else.
const marker = `UPDATE organization SET legal_name = $2 WHERE id = $1`

// statementReadingCase is one synthetic package and the verdict the reader owes
// it.
type statementReadingCase struct {
	name   string
	source string
	// elsewhere is a second synthetic package the first one imports, for the
	// cases where the statement is held across a package boundary.
	elsewhere string
	judged    bool
}

var statementReadingCases = []statementReadingCase{
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
		// The same defect one line lower, and the shape five functions in
		// this tree already write: a `+` chain assembled at the call site
		// rather than at package level. Folding only the hoisted spelling
		// would have fixed one half of one shape.
		name: "assembled in the body from two literals",
		source: `package p
func write(tx T) { tx.Exec(ctx, ` + "`UPDATE organization SET legal_name = $2 `" + ` + ` + "`WHERE id = $1`" + `) }`,
		judged: true,
	}, {
		name: "assembled in the body around a helper's output",
		source: `package p
func write(tx T) { tx.Exec(ctx, ` + "`UPDATE organization SET legal_name = $2 WHERE id = $1 AND `" + ` + extra()) }`,
		judged: true,
	}, {
		// The third spelling of the same assembly, and the one
		// anchorOrganization uses to append the row lock that guards the
		// company form — so a reader blind to it cannot see the guard
		// either, not just the statement.
		name: "appended to a local with +=",
		source: `package p
func write(tx T) {
	statement := ` + "`UPDATE organization SET legal_name = $2 `" + `
	statement += ` + "`WHERE id = $1`" + `
	tx.Exec(ctx, statement)
}`,
		judged: true,
	}, {
		name: "appended to a var declared with a keyword",
		source: `package p
func write(tx T, cond bool) {
	var statement = ` + "`UPDATE organization SET legal_name = $2 `" + `
	if cond {
		statement += ` + "`WHERE id = $1`" + `
	}
	tx.Exec(ctx, statement)
}`,
		judged: true,
	}, {
		// An accumulation that let an inner declaration outlive its block
		// would fold this to "SELECT 1WHERE id = $1" — no SET clause, so the
		// census skips the write entirely. Silence again, from the bookkeeping
		// this time rather than from the reading.
		name: "an inner declaration of a name the outer scope is still building",
		source: `package p
func write(tx T, cond bool) {
	statement := ` + "`UPDATE organization SET legal_name = $2 `" + `
	if cond {
		statement := "SELECT 1"
		_ = statement
	}
	statement += ` + "`WHERE id = $1`" + `
	tx.Exec(ctx, statement)
}`,
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
		// A range clause declares its own variables rather than through an
		// assignment below it, so a reader that only opened the scope would
		// leave them out of it and read every loop body as the package's.
		name: "a range value shadowing the package name",
		source: `package p
var held = ` + "`" + marker + "`" + `
func write(tx T, rows []string) { for _, held := range rows { tx.Exec(ctx, held) } }`,
		judged: false,
	}, {
		name: "a range key shadowing the package name",
		source: `package p
var held = ` + "`" + marker + "`" + `
func write(tx T, rows map[string]int) { for held := range rows { tx.Exec(ctx, held) } }`,
		judged: false,
	}, {
		// The range EXPRESSION stands outside the loop's own scope, so a
		// package statement ranged over is still the package's.
		name: "the package value ranged over",
		source: `package p
var held = []string{` + "`" + marker + "`" + `}
func write(tx T) { for _, q := range held { tx.Exec(ctx, q) } }`,
		judged: true,
	}, {
		// Three identifiers that are not variable references at all. Each
		// would be looked up in the package's statements by a reader that
		// treated every ast.Ident alike, and each would credit a function
		// that sends nothing — a finding that teaches the next author to
		// distrust this census.
		name: "a struct field, a label and a selector spelled like the package name",
		source: `package p
type row struct{ held string }
var held = ` + "`" + marker + "`" + `
func write(tx T, v row) {
held:
	for {
		_ = row{held: "SELECT 1"}
		_ = v.held
		break held
	}
}`,
		judged: false,
	}, {
		name: "a package-level name this function never mentions",
		source: `package p
var held = ` + "`" + marker + "`" + `
func write(tx T) { tx.Exec(ctx, "SELECT 1") }`,
		judged: false,
	}, {
		// The same silence one import away. Closing it inside a package
		// and leaving it across one would have moved the blind spot rather
		// than removed it.
		name: "held in an imported package and sent under a qualified name",
		source: `package p
func write(tx T) { tx.Exec(ctx, storekit.HeldRename) }`,
		elsewhere: `package storekit
const HeldRename = ` + "`" + marker + "`",
		judged: true,
	}, {
		// A field access spelled like a name an imported package happens to
		// hold is not a send of it — but the reader cannot tell them apart
		// without types, so it reads too widely on purpose. Recorded as the
		// over-report it is, in the direction that raises a finding rather
		// than swallowing one.
		name: "a field access colliding with an imported statement name",
		source: `package p
func write(tx T, v row) { _ = v.HeldRename; tx.Exec(ctx, "SELECT 1") }`,
		elsewhere: `package storekit
const HeldRename = ` + "`" + marker + "`",
		judged: true,
	},
}

func TestTheGuardCensusJudgesEveryStatementAFunctionAnswersFor(t *testing.T) {
	t.Parallel()
	for _, tc := range statementReadingCases {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "synthetic.go", tc.source, 0)
			if err != nil {
				t.Fatalf("parsing the case source: %v", err)
			}
			held := packageLevelStatements([]*ast.File{file})
			var imported []map[string][]string
			if tc.elsewhere != "" {
				other, otherErr := parser.ParseFile(fset, "elsewhere.go", tc.elsewhere, 0)
				if otherErr != nil {
					t.Fatalf("parsing the imported case source: %v", otherErr)
				}
				imported = append(imported, packageLevelStatements([]*ast.File{other}))
			}
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
			for _, statement := range statementsJudged(fn, held, imported) {
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
