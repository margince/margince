// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every object the maskable-field catalog offers is one some record's HISTORY
// withholds too.
//
// A mask is configured on an OBJECT and an audit row is filed under an ENTITY
// TYPE, and the two are not always the same word: a partner's terms are
// audited onto the company that carries them, so the tier withheld from the
// partner register is served in full by the company's trail unless auth
// declares the partner a facet of the company. Hiding a value and hiding its
// past is one motion, and the half that fails silently is the past.
//
// Both halves are DERIVED — the catalog says what may be masked, privacy says
// whose history is readable — because a gate that respelled either would guard
// a narrower subject than the tree holds.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	// historyTypesPackage owns which record types have a readable history.
	historyTypesPackage = "internal/modules/privacy"
	historyTypesFile    = historyTypesPackage + "/fieldhistory.go"
	historyTypesVar     = "fieldHistoryEntityTypes"
)

// maskObjectsOutsideAnyHistory is the register of offered objects whose fields
// no record's trail can carry. Empty, and the emptiness is the finding: every
// maskable object today is either a record with a history of its own or a facet
// of one.
var maskObjectsOutsideAnyHistory = gatekit.Waive(map[string]string{})

func TestEveryOfferedMaskObjectIsWithheldFromSomeRecordsHistory(t *testing.T) {
	t.Parallel()

	offered := catalogObjects(t)
	entityTypes := historyEntityTypes(t)
	if len(offered) == 0 || len(entityTypes) == 0 {
		t.Fatalf("this census read %d offered objects and %d history entity types, so it would "+
			"report a clean result over a tree that withheld nothing", len(offered), len(entityTypes))
	}
	facets := auth.HistoryFacets()
	for _, object := range offered {
		if slices.Contains(entityTypes, object) {
			continue // its own trail, read under its own object
		}
		if slices.ContainsFunc(entityTypes, func(e string) bool {
			return slices.Contains(facets[e], object)
		}) {
			continue
		}
		if maskObjectsOutsideAnyHistory.Waived(t, object) {
			continue
		}
		t.Errorf("%s offers masks on %q, which is neither a record with a history of its own nor "+
			"a declared facet of one: the fields it withholds go out in full from the trail they "+
			"are audited onto, every past value of them included. Declare it in auth's "+
			"auditImageFacets under the entity type its images are written to. History entity "+
			"types: %s", maskableCatalog, object, strings.Join(entityTypes, ", "))
	}
	maskObjectsOutsideAnyHistory.AssertAllMatched(t)
}

// historyEntityTypes reads the record types whose history is readable from the
// module that decides it. A key this cannot resolve is REPORTED rather than
// skipped: a census that drops an entry it did not understand reads a smaller
// set and reports the clean result it never obtained.
func historyEntityTypes(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	consts := stringConstsByPackage(t, fset, []string{historyTypesPackage})[historyTypesPackage]
	file, err := parser.ParseFile(fset, historyTypesFile, nil, 0)
	if err != nil {
		t.Fatalf("reading %s: %v", historyTypesFile, err)
	}
	literal, found := mapLiteralOf(file, historyTypesVar)
	if !found {
		t.Fatalf("%s no longer declares %s, so this census has lost half its subject",
			historyTypesFile, historyTypesVar)
	}
	var types []string
	for _, element := range literal.Elts {
		pair, isPair := element.(*ast.KeyValueExpr)
		if !isPair {
			t.Errorf("%s holds an entry this census cannot read", historyTypesVar)
			continue
		}
		types = append(types, resolvedObject(t, pair.Key, consts, historyTypesFile))
	}
	slices.Sort(types)
	return types
}

// mapLiteralOf answers the composite literal one package-level var is assigned.
func mapLiteralOf(file *ast.File, name string) (*ast.CompositeLit, bool) {
	for _, decl := range file.Decls {
		gen, isGen := decl.(*ast.GenDecl)
		if !isGen || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			value, isValue := spec.(*ast.ValueSpec)
			if !isValue || len(value.Names) != 1 || value.Names[0].Name != name || len(value.Values) != 1 {
				continue
			}
			literal, isLiteral := value.Values[0].(*ast.CompositeLit)
			return literal, isLiteral
		}
	}
	return nil, false
}
