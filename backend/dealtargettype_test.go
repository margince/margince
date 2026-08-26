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
	found, dealSites := 0, 0

	// Parsed once, then read twice: the constants must ALL be collected before
	// any identifier is resolved, or a staging in a file walked before the one
	// declaring its constant resolves to nothing and passes.
	var files []*ast.File
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		parsed, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		files = append(files, parsed)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	consts := composeStringConsts(files)

	for _, parsed := range files {
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
				// Only the ONE approved spelling passes. Reporting just the
				// bare literal let every other drift through — a second
				// constant, an alias, a value assigned after construction —
				// which is the same regression under a different spelling.
				if ident, isIdent := kv.Value.(*ast.Ident); isIdent && ident.Name == dealTargetConstName {
					dealSites++
				}
				if spellsDealLiterally(kv.Value, consts) {
					offenders = append(offenders, fset.Position(kv.Pos()).String()+
						" ("+exprText(fset, kv.Value)+")")
				}
			}
			return true
		})
	}

	// A scan that found no DEAL staging read a smaller tree than it thinks and
	// would report PASS with nothing examined. Counting every TargetType would
	// not do: the gate stayed green if every deal staging vanished while an
	// unrelated target remained.
	if dealSites == 0 {
		t.Fatalf("no staging naming approvalTargetDeal found under internal/compose "+
			"(%d TargetType fields seen) — this gate read a smaller tree than it thinks", found)
	}
	for _, at := range offenders {
		t.Errorf("%s does not name its target through a shared constant — use "+
			"approvalTargetDeal, so a proposal cannot be filed against a target "+
			"the inbox fails to resolve", at)
	}
}

// spellsDealLiterally reports whether this value is the deal target written as
// anything other than the one shared constant.
//
// It resolves an identifier through the package's own string constants, so a
// SECOND constant carrying "deal" is caught too — a gate that only rejected the
// bare literal would wave through the same drift under a new name, which is
// what it exists to stop. A forwarded field or a conversion from another
// target's vocabulary resolves to neither and is left alone.
func spellsDealLiterally(v ast.Expr, consts map[string]string) bool {
	switch e := v.(type) {
	case *ast.BasicLit:
		return e.Value == `"deal"`
	case *ast.Ident:
		return e.Name != dealTargetConstName && consts[e.Name] == "deal"
	}
	return false
}

// dealTargetConstName is the one name a deal staging may use.
const dealTargetConstName = "approvalTargetDeal"

// composeStringConsts maps every package-level string constant under
// internal/compose to its value, so an identifier can be resolved.
func composeStringConsts(files []*ast.File) map[string]string {
	out := map[string]string{}
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range value.Names {
					if i >= len(value.Values) {
						continue
					}
					if lit, isLit := value.Values[i].(*ast.BasicLit); isLit && lit.Kind == token.STRING {
						out[name.Name] = strings.Trim(lit.Value, `"`)
					}
				}
			}
		}
	}
	return out
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
