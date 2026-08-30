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
	"strconv"
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

// packageSource parses one directory's non-test Go and indexes the
// package-level string constants it declares.
func packageSource(t *testing.T, dir string) (map[string]*ast.File, map[string]string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	files := map[string]*ast.File{}
	constants := map[string]string{}
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
		for _, decl := range file.Decls {
			spec, isGen := decl.(*ast.GenDecl)
			if !isGen || (spec.Tok != token.CONST && spec.Tok != token.VAR) {
				continue
			}
			for _, s := range spec.Specs {
				value, isValue := s.(*ast.ValueSpec)
				if !isValue {
					continue
				}
				for i, ident := range value.Names {
					if i >= len(value.Values) {
						continue
					}
					if text, ok := stringValue(value.Values[i]); ok {
						constants[ident.Name] = text
					}
				}
			}
		}
	}
	return files, constants
}

// principalActorIDs names the ids a package's principals are built with.
//
// It reads the ID FIELD, not the package's strings: `connector:` appears in
// provenance writers, in prefix tests and in SQL, and none of those binds an
// actor. What this is about is who a piece of work is recorded as.
//
// An id given as an identifier is FOLDED through the package's own string
// constants, because a constant is written down rather than read from
// anywhere: moving the literal one line away does not make it derived.
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
				if text, ok := stringValue(field.Value); ok {
					out = append(out, writtenActorID{file: path, id: text})
					continue
				}
				if ident, isIdent := field.Value.(*ast.Ident); isIdent {
					if text, declared := constants[ident.Name]; declared {
						out = append(out, writtenActorID{file: path, id: text})
					}
				}
				// Anything else — a concatenation, a call, a parameter — is a
				// value the caller read from somewhere, which is the shape
				// this gate is asking for.
			}
			return true
		})
	}
	return out
}

// stringValue is the text of a string literal, or false for anything else.
func stringValue(expr ast.Expr) (string, bool) {
	lit, isLit := expr.(*ast.BasicLit)
	if !isLit || lit.Kind != token.STRING {
		return "", false
	}
	text, err := strconv.Unquote(lit.Value)
	return text, err == nil
}

// namesAPrincipal reports whether a composite literal builds a principal,
// including one taken by address.
func namesAPrincipal(expr ast.Expr, qualifier string, dotImported bool) bool {
	if star, isStar := expr.(*ast.StarExpr); isStar {
		return namesAPrincipal(star.X, qualifier, dotImported)
	}
	return isPrincipalType(expr, qualifier, dotImported)
}
