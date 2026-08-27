// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"go/ast"
	"go/token"
	"strings"
	"testing"
)

// The provenance filter fails by being DROPPED, not by being spelled twice.
// Every list surface assembles its WHERE through a listFilters literal, and a
// literal that omits CapturedByKind hands back an unfiltered page to a caller
// who did ask to filter — the same confident-wrong-answer capturedByKindClause
// refuses an empty enum value to avoid, arriving one layer up where nothing
// looks at the value at all.
//
// Silent at every layer: the request validates, the query runs, the page is
// well-formed, and the only symptom is rows the caller asked not to see. The
// review lists are exactly where that is hardest to notice, because a caller
// filtering for agent-created rows cannot tell "no AI wrote here" from "the
// filter was ignored".
//
// The literals are found rather than listed. A list surface added tomorrow is
// judged by this without anyone remembering to add it here, which is the whole
// difference between a gate and a checklist.

// provenanceField is the filter this holds: the field a listFilters literal
// must carry from its caller.
const provenanceField = "CapturedByKind"

func TestEveryListFiltersLiteralCarriesTheProvenanceFilter(t *testing.T) {
	literals := listFiltersLiterals(t)
	if len(literals) == 0 {
		t.Fatal("no listFilters literal found in this module, so this gate judged nothing — " +
			"either the struct was renamed or the list surfaces have moved")
	}
	for _, lit := range literals {
		if lit.sets[provenanceField] {
			continue
		}
		t.Errorf("%s builds a listFilters without %s.\n\nThe surface then answers a request that "+
			"asked to filter on provenance with a page that did not, and nothing anywhere "+
			"refuses: set it from the caller's input, or give the field the zero value "+
			"explicitly beside a reason this surface has no provenance to filter on.",
			lit.where, provenanceField)
	}
}

// The other half of the claim: one spelling means one caller. A second place
// building the clause is how the person list and the lead list come to disagree
// about which prefix counts as an AI — the two are read side by side, so the
// disagreement reaches a person before it reaches a test.
func TestTheProvenanceClauseIsBuiltInOnePlace(t *testing.T) {
	callers := map[string]int{}
	forEachModuleFile(t, func(name string, fset *token.FileSet, file *ast.File) {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "capturedByKindClause" {
				callers[fset.Position(call.Pos()).String()]++
			}
			return true
		})
	})
	if len(callers) == 0 {
		t.Fatal("capturedByKindClause has no caller in this module — a provenance filter nothing " +
			"builds is a filter every list silently ignores")
	}
	if len(callers) > 1 {
		sites := make([]string, 0, len(callers))
		for site := range callers {
			sites = append(sites, site)
		}
		t.Errorf("capturedByKindClause is called from %d places (%s).\n\nOne caller is what makes "+
			"it the one spelling: every list reaches the prefix rule through listFilters, so a "+
			"second caller is a surface that has started deciding for itself which prefix counts "+
			"as an AI.", len(callers), strings.Join(sites, ", "))
	}
}

// listFiltersLiteral is one composite literal building the shared filter set.
type listFiltersLiteral struct {
	where string
	sets  map[string]bool
}

func listFiltersLiterals(t *testing.T) []listFiltersLiteral {
	t.Helper()
	var found []listFiltersLiteral
	forEachModuleFile(t, func(name string, fset *token.FileSet, file *ast.File) {
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			id, ok := lit.Type.(*ast.Ident)
			if !ok || id.Name != "listFilters" {
				return true
			}
			sets := map[string]bool{}
			for _, element := range lit.Elts {
				kv, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if key, ok := kv.Key.(*ast.Ident); ok {
					sets[key.Name] = true
				}
			}
			found = append(found, listFiltersLiteral{
				where: fset.Position(lit.Pos()).String(), sets: sets,
			})
			return true
		})
	})
	return found
}
