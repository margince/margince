// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A report's answer is labelled with the instant it was COMPUTED at.
//
// The frame carries one AsOf, read from the database, and the money expressions
// convert against it: an open deal takes the latest exchange rate on or before
// that day. The response then states an as_of beside the figures, and a reader
// checking a total reproduces it by asking for that instant.
//
// Those are the same instant or the label is a lie. They were not: the outcome
// took time.Now() at assembly, which is however long the query took AFTER the
// conversion happened, and a rate sheet effective in that gap makes the caption
// name a moment the arithmetic never used. The gap is small and the defect is
// not — it is unreproducible by construction, which is exactly what a caption
// on a revenue figure exists to prevent.
//
// This gate reads the assignment rather than the behaviour, because the
// behaviour needs a rate sheet changing mid-transaction to observe.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// The file and the struct field whose value must come from the frame.
const (
	reportOutcomeFile  = "internal/compose/report.go"
	reportOutcomeField = "GeneratedAt"
	reportFrameSource  = "frame.AsOf"
)

func TestAReportIsLabelledWithTheInstantItWasComputedAt(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, reportOutcomeFile, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", reportOutcomeFile, err)
	}

	var found []string
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok || key.Name != reportOutcomeField {
				continue
			}
			found = append(found, exprText(fset, kv.Value))
		}
		return true
	})

	// A census that finds nothing has failed, not passed: the field could have
	// been renamed and this gate would go on reporting green over a report
	// whose label nobody checks.
	if len(found) == 0 {
		t.Fatalf("no %s assignment found in %s — the gate is reading the wrong place",
			reportOutcomeField, reportOutcomeFile)
	}
	for _, value := range found {
		if value != reportFrameSource {
			t.Errorf("%s is assigned %s, want %s.\n\n"+
				"The answer must be labelled with the instant its money was CONVERTED at. "+
				"A clock read at assembly is later by however long the query took, and a rate "+
				"sheet effective in that gap makes the caption name a moment the arithmetic "+
				"never used — a total nobody can reproduce from the label beside it.",
				reportOutcomeField, value, reportFrameSource)
		}
	}
}
