// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The close-date rejection memory has exactly ONE enforcement point.
//
// StageUnlessDeclined offers a second: declare an Identity and it refuses a
// matching proposal itself, by jsonb containment. That refusal cannot express
// this memory's rule, which compares TONIGHT's standing date against the date an
// EARLIER payload proposed — a relation between two different fields of two
// different rows, not a containment.
//
// So a containment identity here would always be CRUDER than the real judgment,
// and the cruder one wins silently: it suppresses a card RefusedCloseDate had
// already decided to raise, and nothing fails to say so. That was the actual
// defect this file was written after — the sweep stopped telling reps their own
// stale dates, and every unit test still passed.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestTheCloseDateStagingDeclaresNoContainmentIdentity(t *testing.T) {
	t.Parallel()
	const file = "closedate.go"
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", file, err)
	}

	stagings := stageInputLiterals(parsed)
	if len(stagings) == 0 {
		t.Fatalf("%s builds no approvals.StageInput, so this test is watching nothing — "+
			"if the staging moved, move this with it", file)
	}
	for _, staging := range stagings {
		for _, field := range staging.Elts {
			pair, ok := field.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			if name, ok := pair.Key.(*ast.Ident); ok && name.Name == "Identity" {
				t.Error("the close-date staging declares an Identity, which makes " +
					"StageUnlessDeclined refuse by jsonb containment. Containment cannot " +
					"express this memory's rule, so it can only be cruder than " +
					"RefusalProbe.SameQuestionAs — and it silently suppresses cards that " +
					"judgment would have raised")
			}
		}
	}
}

// stageInputLiterals returns EVERY approvals.StageInput composite literal in the
// file, not the first: a second staging added later must be watched too, and a
// gate that stops at one would report PASS over it. Reading the real syntax
// rather than searching for a string is what makes the absence above a fact
// about the code.
func stageInputLiterals(file *ast.File) []*ast.CompositeLit {
	var found []*ast.CompositeLit
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		selector, ok := lit.Type.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "StageInput" {
			return true
		}
		if pkg, ok := selector.X.(*ast.Ident); ok && pkg.Name == "approvals" {
			found = append(found, lit)
		}
		return true
	})
	return found
}
