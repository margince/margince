// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H1

package gates

// A connector's actor id is DERIVED from the work, never written down.
//
// Every value a provider run writes is bought from a vendor rather than typed
// by anybody, so the audit row names the connector — and WHICH connector is a
// fact about the run. It was written down instead: `connector:surfe`, bound by
// the workers that execute a run, correct only while provider_connection,
// provider_run and person_provider_claim each carried a CHECK pinning them to
// one provider. Those checks are gone so a second vendor can be connected.
//
// A LITERAL IS NOT A BUG UNTIL THERE ARE TWO, which is exactly why it needs a
// gate: nothing failed while one vendor was the only one, the claim rows had
// already been made to derive their provenance from the run's own provider, and
// the disagreement appears for the first time on the first installation to
// connect a second. An audit entry naming the wrong actor is worse than a
// missing one, because it reads as authoritative.
//
// So: no principal anywhere in the tree may take a `connector:` id from a
// string literal. Assembling one from a value is how a caller says it read
// which vendor this is.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// connectorActorPrefix is what a connector's actor id starts with, matching the
// provenance written onto the rows the same act produces.
const connectorActorPrefix = "connector:"

// oneOfItsKindForever names the written-down connector ids that are not a
// vendor at all, and so have nothing to derive from.
//
// The rule is about work whose vendor is chosen at RUN TIME. A subsystem that
// records itself as a connector because its values are not typed by anybody is
// the same actor on every installation and every pass, and assembling that name
// from somewhere would be indirection with no second case behind it.
//
// What would make an entry here wrong is the thing that made connector:surfe
// wrong: the name becoming a choice. AssertAllMatched keeps the list honest
// about existing; a reader who finds one of these selecting among vendors
// should delete the entry rather than widen it.
var oneOfItsKindForever = gatekit.Waive(map[string]string{
	"connector:finance": "the finance mirror's own name, not a vendor's: one ledger sweep per installation, converting the source ledger into the base currency as it writes. There is nothing to read the name from, because there is never more than one",
})

// TestNoPrincipalNamesAConnectorInAStringLiteral is the census.
func TestNoPrincipalNamesAConnectorInAStringLiteral(t *testing.T) {
	t.Parallel()
	read := 0
	err := filepath.WalkDir("internal", func(dir string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		// A package at a time, because an id can be a const declared in one
		// file and used in another. Folding those is the whole reason this is
		// not a per-file walk: a const is written down, not read from
		// anywhere, so `const vendor = "connector:surfe"` used as an ID is the
		// same defect one line further away.
		files, constants := packageSource(t, dir)
		for _, named := range principalActorIDs(t, files, constants) {
			read++
			if !strings.HasPrefix(named.id, connectorActorPrefix) || oneOfItsKindForever.Waived(t, named.id) {
				continue
			}
			t.Errorf("%s: builds a principal whose id is the literal %q, so every run this actor is bound for is "+
				"audited as that vendor whichever one it is actually for. Read the provider from the work and "+
				"assemble the id from it — the rows the same act writes already derive their provenance that way",
				named.file, named.id)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	oneOfItsKindForever.AssertAllMatched(t)
	// A tree where no principal carries a written-down id at all would read
	// exactly like a clean one. There are several, all of them named after a
	// job or a subsystem rather than a vendor.
	if read == 0 {
		t.Fatal("this census found no principal built with a literal id, and there are several in this tree: " +
			"the reader has stopped matching rather than the tree having changed, and it cannot tell the difference")
	}
}

// writtenActorID is one principal id this census could read, and where.
type writtenActorID struct {
	file string
	id   string
}

// packageSource parses one directory's non-test Go and indexes every string
// value it binds to a name — package constants and variables, const-block
// repetitions, aliases of either, and function-local bindings alike.
//
// ALL of them, because the defect this gate exists to stop is a connector name
// written down, and every one of those shapes is a name with a literal behind
// it. A package-level const, a `vendor := "connector:surfe"` two lines above
// the principal, and a const aliasing another const are the same thing at
// different distances.
//
// Names are indexed per PACKAGE, not per scope, so two functions binding the
// same name to different strings share an entry. EVERY binding is kept and a
// connector value wins the entry, because the alternative is the failure a
// census may not have: a later `vendor := someProvider` in an unrelated
// function masking an earlier `vendor := "connector:surfe"`, and the literal
// disappearing. Keeping the connector value can only report it at a site that
// did not use it — a loud failure a reader resolves in a minute — where the
// other direction is silent.
func packageSource(t *testing.T, dir string) (map[string]*ast.File, map[string]string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	files := map[string]*ast.File{}
	// bound is name → every expression bound to it, resolved to text afterwards
	// so an alias can name a constant declared later or in another file of the
	// package.
	bound := map[string][]ast.Expr{}
	for _, entry := range entries {
		name := entry.Name()
		path := filepath.ToSlash(filepath.Join(dir, name))
		if entry.IsDir() || !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") || isIntegrationTagged(path) {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		files[path] = file
		indexBindings(file, bound)
	}
	// A fixpoint, because an alias resolves only once its target has: the first
	// pass folds the literals, the next folds the names pointing at them, and
	// so on until a pass changes nothing. No pass count — a cap would truncate
	// a legitimately deep chain and drop its tail out of the census without a
	// sound, and "nothing changed" already ends both a cycle and a chain.
	constants := map[string]string{}
	for {
		progressed := false
		for name, exprs := range bound {
			for _, expr := range exprs {
				text, ok := gatekit.StringExpr(expr, constants, gatekit.FoldStrict)
				if !ok {
					continue
				}
				current, have := constants[name]
				if have && (current == text || strings.HasPrefix(current, connectorActorPrefix)) {
					// Already settled, or already holding the value that must
					// not be masked.
					continue
				}
				if have && !strings.HasPrefix(text, connectorActorPrefix) {
					continue
				}
				constants[name] = text
				progressed = true
			}
		}
		if !progressed {
			break
		}
	}
	return files, constants
}

// indexBindings records every name a file binds to an expression, wherever it
// binds it.
func indexBindings(file *ast.File, bound map[string][]ast.Expr) {
	// A const block repeats the previous spec's values when a spec has none:
	//	const ( vend = "connector:surfe"; alias )
	// carries the same value, and reading only the specs with values would
	// miss it.
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.GenDecl:
			var carried []ast.Expr
			for _, spec := range node.Specs {
				value, isValue := spec.(*ast.ValueSpec)
				if !isValue {
					continue
				}
				values := value.Values
				if len(values) == 0 {
					values = carried
				} else {
					carried = values
				}
				for i, ident := range value.Names {
					if i < len(values) {
						bound[ident.Name] = append(bound[ident.Name], values[i])
					}
				}
			}
		case *ast.AssignStmt:
			for i, lhs := range node.Lhs {
				ident, isIdent := lhs.(*ast.Ident)
				if isIdent && i < len(node.Rhs) {
					bound[ident.Name] = append(bound[ident.Name], node.Rhs[i])
				}
			}
		}
		return true
	})
}

// principalActorIDs names the ids a package's principals are built with.
//
// It reads the ID FIELD, not the package's strings: `connector:` appears in
// provenance writers, in prefix tests and in SQL, and none of those binds an
// actor. What this is about is who a piece of work is recorded as.
//
// An id given as an identifier is FOLDED through every string the package
// binds to a name — a const, a var, a const-block repetition, an alias of any
// of those, or a local two lines up. All of them are a connector name written
// down, and moving the literal further away does not make it derived.
//
// An UNKEYED principal literal is refused outright. This reader matches the
// field by name, and a positional one sets ID with nothing to match on —
// answering "no id here" about a literal that carries one is the
// under-recognition a census may not have.
func principalActorIDs(t *testing.T, files map[string]*ast.File, constants map[string]string) []writtenActorID {
	t.Helper()
	var out []writtenActorID
	for path, file := range files {
		qualifier, dotImported := gatekit.ImportedAs(file, principalImportPath)
		if qualifier == "" && !dotImported {
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, isLit := n.(*ast.CompositeLit)
			if !isLit || !namesAPrincipal(lit.Type, qualifier, dotImported) {
				return true
			}
			// Once per literal, not once per element: a positional principal
			// has as many unkeyed elements as it has fields, and nine copies
			// of one sentence is not nine findings.
			if len(lit.Elts) > 0 {
				if _, keyed := lit.Elts[0].(*ast.KeyValueExpr); !keyed {
					t.Errorf("%s: builds a principal with unkeyed fields, and this census reads the ID field by "+
						"name — so an actor id spelled positionally is one it cannot see. Name the fields", path)
					return true
				}
			}
			for _, element := range lit.Elts {
				field, isField := element.(*ast.KeyValueExpr)
				if !isField {
					continue
				}
				if key, isIdent := field.Key.(*ast.Ident); !isIdent || key.Name != "ID" {
					continue
				}
				// FoldStrict: "is this DEFINITELY this string". Anything it
				// cannot settle — a concatenation with a parameter in it, a
				// call, a value read from a row — is the shape this gate is
				// asking for, and reporting a guess about one would be worse
				// than saying nothing.
				if text, ok := gatekit.StringExpr(field.Value, constants, gatekit.FoldStrict); ok {
					out = append(out, writtenActorID{file: path, id: text})
				}
			}
			return true
		})
	}
	return out
}

// namesAPrincipal reports whether a composite literal builds a principal,
// including one taken by address.
func namesAPrincipal(expr ast.Expr, qualifier string, dotImported bool) bool {
	if star, isStar := expr.(*ast.StarExpr); isStar {
		return namesAPrincipal(star.X, qualifier, dotImported)
	}
	return isPrincipalType(expr, qualifier, dotImported)
}
