// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// The site-lead lane is open exactly when accepting a lead sends the person an
// Article 14 notice. Read off the accept path's own source, so the switch and
// the step it waits for cannot drift apart: opening the lane without the step
// fails here, and so does building the step while the lane stays shut.
func TestSiteLeadCaptureOpensOnlyWithANoticeStep(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "siteleadaccept.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing the accept path: %v", err)
	}
	var effect *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "siteLeadAcceptEffect" {
			effect = fn
		}
	}
	if effect == nil {
		t.Fatal("siteleadaccept.go declares no siteLeadAcceptEffect; this gate reads the accept path there")
	}
	var noticeCalls []string
	ast.Inspect(effect, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if name := calleeName(call.Fun); strings.Contains(strings.ToLower(name), "article14") {
			noticeCalls = append(noticeCalls, name)
		}
		return true
	})
	sendsNotice := len(noticeCalls) > 0
	if siteLeadCaptureOpen && !sendsNotice {
		t.Error("siteLeadCaptureOpen is true but siteLeadAcceptEffect calls no Article 14 notice step: " +
			"a person named on a website would be captured with no notice")
	}
	if !siteLeadCaptureOpen && sendsNotice {
		t.Errorf("siteLeadAcceptEffect calls %v but siteLeadCaptureOpen is still false: "+
			"the notice path exists, so open the lane and delete the refusals it guards", noticeCalls)
	}
}

// calleeName is the name a call is made through: a bare function, or the
// selector of a method or package function.
func calleeName(fun ast.Expr) string {
	switch callee := fun.(type) {
	case *ast.Ident:
		return callee.Name
	case *ast.SelectorExpr:
		return callee.Sel.Name
	default:
		return ""
	}
}
