// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// noticeStep is the one name the accept path must call, directly, before the
// lane may open. One exact name rather than a pattern, so a helper merely
// mentioning the article does not count.
const noticeStep = "sendArticle14Notice"

// The site-lead lane is open exactly when accepting a lead sends its subject an
// Article 14 notice, read off the accept path's own source: opening the lane
// without the step fails here, and so does building the step while the lane
// stays shut.
func TestSiteLeadCaptureOpensOnlyWithANoticeStep(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "siteleadaccept.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing the accept path: %v", err)
	}
	sendsNotice, found := acceptPathSendsNotice(file)
	if !found {
		t.Fatal("siteleadaccept.go declares no siteLeadAcceptEffect returning a closure; this gate reads the accept path there")
	}
	if siteLeadCaptureOpen && !sendsNotice {
		t.Errorf("siteLeadCaptureOpen is true but siteLeadAcceptEffect does not call %s directly in its body: "+
			"somebody named on a website would be captured with no notice", noticeStep)
	}
	if !siteLeadCaptureOpen && sendsNotice {
		t.Errorf("siteLeadAcceptEffect calls %s but siteLeadCaptureOpen is still false: "+
			"the notice path exists, so open the lane and delete the refusals it guards", noticeStep)
	}
}

// The checker itself, on shapes that must and must not count, so a gate that
// silently stops recognising the step fails here rather than passing on nothing.
func TestTheNoticeCheckCountsOnlyADirectCallOfTheStep(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want bool
	}{
		{"a direct call", `sendArticle14Notice(ctx)`, true},
		{"a checked direct call", `if err := sendArticle14Notice(ctx); err != nil { return err }`, true},
		{"an assigned direct call", `err := sendArticle14Notice(ctx); _ = err`, true},
		{"a call that can never run", `if false { sendArticle14Notice(ctx) }`, false},
		{"a different name", `article14Required(ctx)`, false},
		{"a call in a nested closure", `go func() { sendArticle14Notice(ctx) }()`, false},
		{"no call at all", `return nil`, false},
	} {
		src := "package p\nfunc siteLeadAcceptEffect() func() error {\n\treturn func() error {\n\t\t" +
			tc.body + "\n\t\treturn nil\n\t}\n}\n"
		file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", src, 0)
		if err != nil {
			t.Fatalf("%s: parsing the fixture: %v", tc.name, err)
		}
		got, found := acceptPathSendsNotice(file)
		if !found || got != tc.want {
			t.Errorf("%s: sends notice = %v (accept path found %v), want %v", tc.name, got, found, tc.want)
		}
	}
}

// acceptPathSendsNotice reports whether the closure siteLeadAcceptEffect
// returns calls noticeStep as one of its own top-level statements, and whether
// that closure was found at all.
func acceptPathSendsNotice(file *ast.File) (sends, found bool) {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "siteLeadAcceptEffect" || fn.Body == nil {
			continue
		}
		for _, stmt := range fn.Body.List {
			ret, ok := stmt.(*ast.ReturnStmt)
			if !ok || len(ret.Results) != 1 {
				continue
			}
			closure, ok := ret.Results[0].(*ast.FuncLit)
			if !ok {
				continue
			}
			for _, top := range closure.Body.List {
				if statementCallsDirectly(top, noticeStep) {
					return true, true
				}
			}
			return false, true
		}
	}
	return false, false
}

// statementCallsDirectly reports whether stmt itself calls name: as an
// expression statement, the right side of an assignment, or an if statement's
// init. A call inside any nested block does not count.
func statementCallsDirectly(stmt ast.Stmt, name string) bool {
	switch s := stmt.(type) {
	case *ast.ExprStmt:
		return isCallOf(s.X, name)
	case *ast.AssignStmt:
		for _, rhs := range s.Rhs {
			if isCallOf(rhs, name) {
				return true
			}
		}
	case *ast.IfStmt:
		return s.Init != nil && statementCallsDirectly(s.Init, name)
	}
	return false
}

func isCallOf(expr ast.Expr, name string) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	ident, ok := call.Fun.(*ast.Ident)
	return ok && ident.Name == name
}

// The accept step's own refusal is the backstop behind the precheck, so no path
// through approvals reaches it while the precheck stands. Called directly: it
// must refuse before touching the approval or the capture sink, both nil here.
func TestTheSiteLeadAcceptStepRefusesBeforeItRedeemsOrCaptures(t *testing.T) {
	effect := siteLeadAcceptEffect(nil, nil)
	err := effect(context.Background(), ids.New[ids.ApprovalKind](), []byte(`{}`), "hash")
	if !errors.Is(err, errSiteLeadCaptureClosed) || !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("accepting a site lead while the lane is shut = %v, want the closed-lane conflict", err)
	}
	if siteLeadPrecheck()(context.Background(), nil, nil) == nil {
		t.Fatal("the precheck let a site lead through while the lane is shut")
	}
}

// A read that found nobody has nothing to refuse: it is not reported as a
// refusal, and it logs nothing about one.
func TestSiteLeadsRefusedSaysNothingWhenTheReadFoundNobody(t *testing.T) {
	var logged bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logged, nil))
	if siteLeadsRefused(context.Background(), log, "read-1", 0) {
		t.Error("a read that found nobody was reported as refusing somebody")
	}
	if logged.Len() != 0 {
		t.Errorf("a read that found nobody logged a refusal: %s", logged.String())
	}
	if !siteLeadsRefused(context.Background(), log, "read-2", 2) {
		t.Error("two published names were not refused while the lane is shut")
	}
	if !bytes.Contains(logged.Bytes(), []byte(dropNoArticle14Notice)) {
		t.Errorf("the refusal did not log its reason: %s", logged.String())
	}
}
