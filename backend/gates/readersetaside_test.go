// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H1

package gates

// Every read that hides a message a reader set aside goes through one predicate,
// activities.readerSetAsideSQL.
//
// A new reader state is added to the table's check and to that function. A
// second spelling of the filter elsewhere would miss the new state. The message
// would then leave one list and stay on another for the same reader.
//
// What it sees: every string literal in non-test Go under internal/ and
// extensions/, read as SQL. It flags an EXISTS or IN subquery that selects from
// activity_reader_state, and any JOIN onto it. One such literal may exist,
// inside readerSetAsideSQL.
//
// What it cannot see: SQL built from fragments that split the table name from
// its SELECT. Nor a filter written as a plain top-level SELECT over the table.
// The writers and the undo's top-level reads pass, since none decides what a
// list shows.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

// readerStateFilter matches a statement that filters rows by what a reader set
// aside. It allows no parenthesis inside the subquery head, so one subquery is
// one match.
var readerStateFilter = regexp.MustCompile(
	`(?is)(\b(EXISTS|IN)\s*\(\s*SELECT\b[^()]*?\bFROM\s+activity_reader_state\b|\bJOIN\s+activity_reader_state\b)`)

// sharedSetAsideOwner is where the one permitted filter lives.
const sharedSetAsideOwner = "readerSetAsideSQL"

func TestEveryReaderSetAsideReadIsTheSharedPredicate(t *testing.T) {
	t.Parallel()
	var inOwner int
	for _, root := range []string{"internal", "../extensions"} {
		for path, src := range goSourcesUnder(t, root) {
			file, err := parser.ParseFile(token.NewFileSet(), path, src, parser.SkipObjectResolution)
			if err != nil {
				t.Fatalf("parsing %s: %v", path, err)
			}
			for _, decl := range file.Decls {
				owner := declName(decl)
				ast.Inspect(decl, func(node ast.Node) bool {
					literal, ok := node.(*ast.BasicLit)
					if !ok || literal.Kind != token.STRING {
						return true
					}
					text, err := strconv.Unquote(literal.Value)
					if err != nil || !readerStateFilter.MatchString(text) {
						return true
					}
					if owner == sharedSetAsideOwner && filepath.Base(filepath.Dir(path)) == "activities" {
						inOwner++
						return true
					}
					t.Errorf("%s (in %s) filters on activity_reader_state with its own SQL. "+
						"Embed activities.readerSetAsideSQL instead, so a new reader state "+
						"hides the message from every list at once.", path, owner)
					return true
				})
			}
		}
	}
	// A short census: the owner moved, was renamed, or stopped reading the
	// table, and every other hit above would then have passed unseen.
	if inOwner != 1 {
		t.Errorf("found %d filters on activity_reader_state inside %s, want exactly 1; "+
			"this gate reads its own subject there", inOwner, sharedSetAsideOwner)
	}
}

// declName names a top-level function, or the first name a var or const
// declaration binds, so a finding says where to look.
func declName(decl ast.Decl) string {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		return d.Name.Name
	case *ast.GenDecl:
		for _, spec := range d.Specs {
			if value, ok := spec.(*ast.ValueSpec); ok && len(value.Names) > 0 {
				return value.Names[0].Name
			}
		}
	}
	return "a declaration with no name"
}
