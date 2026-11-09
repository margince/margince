// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H3

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// Which way the PII census fails when it cannot read a statement.
//
// It is a census over an Art. 17 erasure, so the two directions are not
// symmetric: reporting a redaction it cannot read costs somebody a look at one
// statement, and skipping one costs a contact the deletion they asked for while
// the build stays green. Every case here is a shape where the reading runs out,
// and every one of them must fail CLOSED.
func TestAStatementTheCensusCannotReadIsNotReadAsClean(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		statement string
		want      bool
	}{
		"a rendered redaction reads": {"UPDATE contact SET note = NULL WHERE id = $1", true},
		"two rendered clauses read":  {"UPDATE contact SET a = NULL, b = NULL WHERE id = $1", true},
		// A VALUE the reader could not render hides no column: `note` is
		// plainly assigned, and what it is assigned TO is a question this
		// census does not ask of any statement, rendered or not. Refusing here
		// took a fully accounted-for table out of the census for one interval
		// built by a helper.
		"a value built from a const": {"UPDATE contact SET note = " + unresolved + " WHERE id = $1", true},
		// The marker on the LEFT is another matter: there it could be standing
		// over an assignment, and what it clears is what nobody can see.
		"a column built from a const": {"UPDATE contact SET " + unresolved + " = NULL WHERE id = $1", false},
		"a marker among real pairs":   {"UPDATE contact SET a = NULL, " + unresolved + ", b = NULL", false},
		"commas inside a value":       {"UPDATE contact SET a = coalesce(x, ''), b = NULL", true},
		"an array value with commas":  {"UPDATE contact SET a = ARRAY[x, y], b = NULL", true},
		// THE WHOLE CLAUSE swallowed, which is `"UPDATE contact " + clearEverything`.
		// The `set` token goes with it, so the clause pattern finds nothing at
		// all — and answering "readable" for a statement with no readable
		// assignment in it is this census skipping the one write it could not
		// read.
		"the clause swallowed whole": {"UPDATE contact " + unresolved, false},
		"nothing but the marker":     {unresolved, false},
		// Here the value was meant to close its own quote. The rendering never
		// closes it, so the open string runs over the next assignment. The
		// column the second marker stands over then reads as quoted text.
		"a quote the value closes": {"UPDATE contact SET note = '" + unresolved + ", " + unresolved + " = NULL WHERE id = $1", false},
		// A keyword inside a literal is part of the value, so the clause runs
		// on past it to the assignment the marker stands over.
		"a keyword in a literal":        {"UPDATE contact SET note = 'where it came from', " + unresolved + " = NULL WHERE id = $1", false},
		"a value before a quoted where": {"UPDATE contact SET note = " + unresolved + ", reason = 'where' WHERE id = $1", true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := assignmentsAreReadable(tc.statement); got != tc.want {
				t.Errorf("assignmentsAreReadable(%q) = %v, want %v", tc.statement, got, tc.want)
			}
		})
	}
}

// And which writes count as redacting a table at all.
//
// `writeTarget` matches `INSERT INTO` too — an insert's ON CONFLICT can carry an
// UPDATE, so it has to. A write with no SET clause assigns nothing, and
// registering its table as judged demanded a baseline for every text column of a
// table the cascade may never redact.
func TestOnlyAnAssigningWriteRegistersATableAsRedacted(t *testing.T) {
	t.Parallel()
	columns := map[string]map[string]string{"contact": {"note": "text", "full_name": "text"}}
	for name, tc := range map[string]struct {
		statement string
		want      map[string]bool
	}{
		"an update assigns": {
			"UPDATE contact SET note = NULL WHERE id = $1",
			map[string]bool{"note": true},
		},
		"an upsert's DO UPDATE assigns": {
			"INSERT INTO contact (id, note) VALUES ($1, $2) ON CONFLICT (id) DO UPDATE SET note = NULL",
			map[string]bool{"note": true},
		},
		"a plain insert assigns nothing": {
			"INSERT INTO contact (id, note) VALUES ($1, $2)",
			nil,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := erasureRedactedColumns(t, columns, []string{tc.statement})
			if tc.want == nil {
				if _, judged := got["contact"]; judged {
					t.Errorf("a write that assigns nothing registered contact as judged: %v — every "+
						"text column of it would then need a baseline entry", got)
				}
				return
			}
			if len(got["contact"]) != len(tc.want) {
				t.Fatalf("redacted %v, want %v", got["contact"], tc.want)
			}
			for column := range tc.want {
				if !got["contact"][column] {
					t.Errorf("%s was assigned and is not reported as redacted: %v", column, got["contact"])
				}
			}
		})
	}
}

func cascadeSQLForwarders(t *testing.T) map[string]int {
	t.Helper()
	var functions []*ast.FuncDecl
	for _, path := range erasureCascadeFiles {
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			if fn, ok := declaration.(*ast.FuncDecl); ok {
				functions = append(functions, fn)
			}
		}
	}
	return sqlForwardersIn(functions)
}

func sqlForwardersIn(functions []*ast.FuncDecl) map[string]int {
	forwarders := map[string]int{}
	for {
		before := len(forwarders)
		for _, fn := range functions {
			parameters := map[string]int{}
			index := 0
			for _, field := range fn.Type.Params.List {
				for _, name := range field.Names {
					parameters[name.Name] = index
					index++
				}
				if len(field.Names) == 0 {
					index++
				}
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				argument, executes := executedSQL(node, forwarders)
				if !executes {
					return true
				}
				name, named := argument.(*ast.Ident)
				if !named {
					return true
				}
				position, parameter := parameters[name.Name]
				if parameter {
					if prior, exists := forwarders[fn.Name.Name]; exists && prior != position {
						forwarders[fn.Name.Name] = -1
					} else {
						forwarders[fn.Name.Name] = position
					}
				}
				return true
			})
		}
		if len(forwarders) == before {
			return forwarders
		}
	}
}

func TestTheColumnCensusFollowsSQLThroughHelperArguments(t *testing.T) {
	t.Parallel()
	source := `package fixture
func second(ctx C, tx T, statement string) { first(ctx, tx, statement) }
func first(ctx C, tx T, statement string) { tx.Query(ctx, statement) }
func single(statement string) { tx.Query(ctx, statement) }
func ambiguous(ctx C, tx T, first, second string) { tx.Query(ctx, first); tx.Query(ctx, second) }
func write(ctx C, tx T) {
 second(ctx, tx, "UPDATE contact SET full_name = ''")
 single("UPDATE contact SET phone = NULL")
 ambiguous(ctx, tx, "UPDATE contact SET a = NULL", "UPDATE contact SET b = NULL")
}
`
	file, err := parser.ParseFile(token.NewFileSet(), "", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	var functions []*ast.FuncDecl
	for _, declaration := range file.Decls {
		if fn, ok := declaration.(*ast.FuncDecl); ok {
			functions = append(functions, fn)
		}
	}
	forwarders := sqlForwardersIn(functions)
	for name, want := range map[string]int{"first": 2, "second": 2, "single": 0, "ambiguous": -1} {
		if got, found := forwarders[name]; !found || got != want {
			t.Fatalf("SQL argument positions = %v; want %s at %d", forwarders, name, want)
		}
	}
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		argument, executes := executedSQL(node, forwarders)
		if executes && renderSQL(argument, nil) == "UPDATE contact SET full_name = ''" {
			found = true
		}
		return true
	})
	if !found {
		t.Fatal("the helper's contact redaction disappeared from the column census")
	}
}

func TestTheColumnCensusDoesNotCountConditionsOrQuotedAssignmentsAsWrites(t *testing.T) {
	t.Parallel()
	for _, statement := range []string{
		`UPDATE intro_request SET internal_reason=CASE WHEN origin='recorded' THEN NULL ELSE '' END`,
		`UPDATE intro_request SET internal_reason='example, origin=recorded'`,
		`UPDATE intro_request SET internal_reason=coalesce(origin, '')`,
	} {
		if assignsColumn(statement, "origin") {
			t.Errorf("a value counted as an origin assignment: %s", statement)
		}
		if !assignsColumn(statement, "internal_reason") {
			t.Errorf("the real assignment disappeared: %s", statement)
		}
	}
	for _, statement := range []string{
		`UPDATE intro_request SET "origin" = ''`,
		`UPDATE intro_request SET ("origin", internal_reason) = ('', '')`,
		`UPDATE intro_request SET internal_reason = 'where it came from', origin = NULL`,
	} {
		if !assignsColumn(statement, "origin") {
			t.Errorf("the real origin assignment disappeared: %s", statement)
		}
	}
}
