// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates_test

// "What survives a reset" is one decision, and it is written down twice: the
// in-product data reset applies it in Go (internal/compose/datasweep.go's
// preservedResetTables), and the developer's `make seed-reset` applies it in SQL
// (scripts/seed-reset.sql). Two answers to that question is how a dev database
// and a customer's diverge quietly — and the divergence is invisible from either
// side, because each is correct on its own terms.
//
// The failure this catches is one-directional in the dangerous way. A table
// added to the Go list but not the SQL one is DELETED by seed-reset and kept by
// the product: a developer loses configuration the product promises to keep, and
// finds out when the stack stops working. A table added to the SQL list but not
// the Go one survives seed-reset and is deleted by the product: the developer's
// database keeps rows a customer's would not, so a bug that only shows on a
// swept database never reproduces locally.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const (
	datasweepFile  = "internal/compose/datasweep.go"
	seedResetFile  = "../scripts/seed-reset.sql"
	preservedIdent = "preservedResetTables"
)

// preservedArrayEntry matches one quoted name inside seed-reset.sql's preserved
// ARRAY[...] literal. SQL has no parser here, so the shape is pinned by the
// count assertion below rather than by trusting the regex alone.
//
// Digits are IN the alphabet, and that is not cosmetic. Without them a name
// like `org360_view` matched as far as its first digit and was read as `org` —
// a name neither list holds — so the gate reported the SQL side preserving
// something the Go side does not AND missing something it does, from one
// correctly mirrored entry. A reader chasing that would have edited a file that
// was already right.
var preservedArrayEntry = regexp.MustCompile(`'([a-z0-9_]+)'`)

func TestTheTwoResetsPreserveTheSameTables(t *testing.T) {
	fromGo := goPreservedTables(t)
	fromSQL := sqlPreservedTables(t)

	// Both corpora asserted before they are compared. Two empty lists are
	// equal, and a gate that passed on them would be reporting agreement
	// between two things it failed to read.
	if len(fromGo) == 0 {
		t.Fatalf("read no preserved tables from %s — the gate is comparing nothing", datasweepFile)
	}
	if len(fromSQL) == 0 {
		t.Fatalf("read no preserved tables from %s — the gate is comparing nothing", seedResetFile)
	}

	for _, name := range fromGo {
		if !slices.Contains(fromSQL, name) {
			t.Errorf("%s preserves %q and %s does not — `make seed-reset` DELETES a table the product keeps, "+
				"so a developer loses what the product promises to preserve", datasweepFile, name, seedResetFile)
		}
	}
	for _, name := range fromSQL {
		if !slices.Contains(fromGo, name) {
			t.Errorf("%s preserves %q and %s does not — a developer's database keeps rows a customer's would not, "+
				"so a bug that only shows on a swept database never reproduces locally", seedResetFile, name, datasweepFile)
		}
	}
}

// goPreservedTables reads the keys of preservedResetTables from the AST, so the
// gate reads the declaration the product actually uses rather than a copy.
func goPreservedTables(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), datasweepFile, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", datasweepFile, err)
	}
	var names []string
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || spec.Names[0].Name != preservedIdent {
			return true
		}
		lit, ok := spec.Values[0].(*ast.CompositeLit)
		if !ok {
			t.Fatalf("%s is not a composite literal", preservedIdent)
		}
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			if !preservedValue(t, kv.Value) {
				continue // set to false: this entry is not preserved
			}
			names = append(names, preservedKeyName(t, kv.Key))
		}
		return false
	})
	slices.Sort(names)
	return names
}

// preservedValue resolves one map value. A false entry means the table is not
// preserved — its key must not enter the corpus, or a table the product
// deletes would still read here as one seed-reset must keep. Any shape other
// than the bool literals fails loudly rather than being treated as either.
func preservedValue(t *testing.T, value ast.Expr) bool {
	t.Helper()
	ident, ok := value.(*ast.Ident)
	if !ok {
		t.Fatalf("%s holds a value shape this gate cannot read: %T", preservedIdent, value)
	}
	switch ident.Name {
	case "true":
		return true
	case "false":
		return false
	}
	t.Fatalf("%s holds a value %q this gate cannot resolve to true or false", preservedIdent, ident.Name)
	return false
}

// preservedKeyName resolves one map key. Most are string literals; the workspace
// entry is the objectWorkspace constant, which is resolved by name rather than
// by value so a rename fails loudly here instead of silently dropping a table.
func preservedKeyName(t *testing.T, key ast.Expr) string {
	t.Helper()
	switch k := key.(type) {
	case *ast.BasicLit:
		name, err := strconv.Unquote(k.Value)
		if err != nil {
			t.Fatalf("%s holds an unreadable key %s", preservedIdent, k.Value)
		}
		return name
	case *ast.Ident:
		value := constantString(t, k.Name)
		if value == "" {
			t.Fatalf("%s is keyed by constant %s, whose value this gate cannot resolve", preservedIdent, k.Name)
		}
		return value
	}
	t.Fatalf("%s holds a key shape this gate cannot read: %T", preservedIdent, key)
	return ""
}

// constantString finds a `const <name> = "..."` in the compose package.
func constantString(t *testing.T, name string) string {
	t.Helper()
	entries, err := os.ReadDir("internal/compose")
	if err != nil {
		t.Fatalf("reading internal/compose: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), "internal/compose/"+entry.Name(), nil, 0)
		if err != nil {
			continue
		}
		var found string
		ast.Inspect(file, func(n ast.Node) bool {
			spec, ok := n.(*ast.ValueSpec)
			if !ok || len(spec.Names) != 1 || spec.Names[0].Name != name || len(spec.Values) != 1 {
				return true
			}
			if lit, ok := spec.Values[0].(*ast.BasicLit); ok {
				if value, err := strconv.Unquote(lit.Value); err == nil {
					found = value
				}
			}
			return false
		})
		if found != "" {
			return found
		}
	}
	return ""
}

// sqlPreservedTables reads the names out of seed-reset.sql's preserved ARRAY.
func sqlPreservedTables(t *testing.T) []string {
	t.Helper()
	source, err := os.ReadFile(seedResetFile)
	if err != nil {
		t.Fatalf("reading %s: %v", seedResetFile, err)
	}
	_, rest, found := strings.Cut(string(source), "preserved text[] := ARRAY[")
	if !found {
		t.Fatalf("%s no longer declares a `preserved text[] := ARRAY[` literal — "+
			"this gate reads that shape, so a different one makes it read nothing", seedResetFile)
	}
	body, _, found := strings.Cut(rest, "]")
	if !found {
		t.Fatalf("%s's preserved ARRAY literal is unterminated", seedResetFile)
	}
	var names []string
	for _, match := range preservedArrayEntry.FindAllStringSubmatch(body, -1) {
		names = append(names, match[1])
	}
	slices.Sort(names)
	return names
}
