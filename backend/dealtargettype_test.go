// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package backendarch

// Every deal-scoped staging names its target type through one constant.
//
// Three stagers file proposals against a deal — the close-date corrector, the
// nightly follow-up task and the drafted reply — and a fourth is cheap to add.
// A target type that disagreed between them files a proposal the inbox cannot
// resolve to a record, so the reader sees a card about nothing and the deal it
// was raised on never hears about it.
//
// Held here rather than by a linter threshold: goconst noticed the third
// occurrence only because a count crossed a number, which would have let the
// first two disagree indefinitely.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEveryStagingNamesItsDealTargetThroughOneConstant(t *testing.T) {
	root := filepath.Join("internal", "compose")
	fset := token.NewFileSet()
	var offenders []string
	found := 0

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		parsed, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok || !isStageInputLit(lit) {
				return true
			}
			for _, elt := range lit.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.Ident)
				if !ok || key.Name != "TargetType" {
					continue
				}
				found++
				// A string literal here is the defect: the value must come
				// through a named constant so the four sites cannot drift.
				if str, isLiteral := kv.Value.(*ast.BasicLit); isLiteral && str.Value == `"deal"` {
					offenders = append(offenders, fset.Position(kv.Pos()).String())
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}

	// A scan that found no staging at all read a smaller tree than it thinks,
	// and would report PASS with nothing examined.
	if found == 0 {
		t.Fatal("no StageInput carrying a TargetType found under internal/compose")
	}
	for _, at := range offenders {
		t.Errorf("%s spells the deal target type as a literal — use approvalTargetDeal, "+
			"so a proposal cannot be filed against a target the inbox fails to resolve", at)
	}
}

// isStageInputLit reports whether this composite literal is an approvals
// StageInput, written either qualified or bare.
func isStageInputLit(lit *ast.CompositeLit) bool {
	switch typ := lit.Type.(type) {
	case *ast.SelectorExpr:
		return typ.Sel.Name == "StageInput"
	case *ast.Ident:
		return typ.Name == "StageInput"
	}
	return false
}
