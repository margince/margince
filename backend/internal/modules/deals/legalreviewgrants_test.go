// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
)

// The answer, the reviewer list and the inbox card name one object. The answer
// spells it as a literal for the table-ownership gate, so this holds the two.
func TestTheAnswerAndTheReviewerListNameOneObject(t *testing.T) {
	if legalReviewEntity != approvalsubject.ObjectLegalReview {
		t.Errorf("deals answers under %q, the reviewer list and card under %q", legalReviewEntity, approvalsubject.ObjectLegalReview)
	}
}

// The answer names its verbs as literals, which the grant scan can read. This
// holds them to the shared declaration.
func TestTheAnswerRequiresTheDeclaredActions(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "legalreviewdecide.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var fn *ast.FuncDecl
	for _, decl := range file.Decls {
		if f, ok := decl.(*ast.FuncDecl); ok && f.Name.Name == "DecideLegalReview" {
			fn = f
		}
	}
	if fn == nil {
		t.Fatal("DecideLegalReview not found")
	}
	var got []string
	ast.Inspect(fn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 3 {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); !ok || sel.Sel.Name != "Require" {
			return true
		}
		if id, ok := call.Args[1].(*ast.Ident); ok && id.Name == "legalReviewEntity" {
			got = append(got, string(call.Args[2].(*ast.SelectorExpr).Sel.Name))
		}
		return true
	})
	var want []string
	for _, a := range approvalsubject.LegalReviewDecisionActions() {
		want = append(want, "Action"+strings.ToUpper(string(a)[:1])+string(a)[1:])
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("the answer requires %v, the declaration holds %v", got, want)
	}
}
