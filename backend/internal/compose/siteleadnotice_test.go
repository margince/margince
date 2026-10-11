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

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// noticeStep is the one name the accept path must call, directly. One exact
// name rather than a pattern, so a helper merely mentioning the article does
// not count.
const noticeStep = "sendArticle14Notice"

// The site-lead lane opens only with a reviewed notice in the accept path.
//
// Two rules, read off the source and the constants. The accept path always
// calls the notice step directly, so no build can capture a website lead
// without telling them. The lane opens only by naming the privacy_notice
// version legal review approved. That must be the version this build sends,
// so new wording shuts the lane until it is reviewed again.
func TestSiteLeadCaptureOpensOnlyWithAReviewedNotice(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "siteleadaccept.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing the accept path: %v", err)
	}
	sendsNotice, found := acceptPathSendsNotice(file)
	if !found {
		t.Fatal("siteleadaccept.go declares no siteLeadAcceptEffect returning a closure; this gate reads the accept path there")
	}
	if !sendsNotice {
		t.Errorf("siteLeadAcceptEffect does not call %s directly in its body: "+
			"somebody named on a website would be captured with no notice", noticeStep)
	}
	if got := newSiteLeadNoticeLane().reviewed; got != siteLeadNoticeReviewed {
		t.Errorf("the production lane sends reviewed version %d, and the review named %d: "+
			"the lane opens by naming the reviewed notice and in no other way", got, siteLeadNoticeReviewed)
	}
	current, ok := consent.ControllerTemplateVersion(consent.TemplatePrivacyNotice)
	if !ok {
		t.Fatal("consent registers no privacy_notice template; a website lead would be told nothing")
	}
	if siteLeadNoticeReviewed != 0 && siteLeadNoticeReviewed != current {
		t.Errorf("legal review approved privacy_notice version %d, and this build sends version %d: "+
			"review the new wording before the lane opens on it", siteLeadNoticeReviewed, current)
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
		{"a returned direct call", `return sendArticle14Notice(ctx)`, true},
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
// expression statement, a returned value, the right side of an assignment, or
// an if statement's init. A call inside any nested block does not count.
func statementCallsDirectly(stmt ast.Stmt, name string) bool {
	switch s := stmt.(type) {
	case *ast.ExprStmt:
		return isCallOf(s.X, name)
	case *ast.ReturnStmt:
		for _, result := range s.Results {
			if isCallOf(result, name) {
				return true
			}
		}
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
	effect := siteLeadAcceptEffect(nil, nil, nil, nil)
	err := effect(context.Background(), ids.New[ids.ApprovalKind](), []byte(`{}`), "hash")
	if !errors.Is(err, errSiteLeadCaptureClosed) || !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("accepting a site lead while the lane is shut = %v, want the closed-lane conflict", err)
	}
	if siteLeadPrecheck(&siteLeadNoticeLane{})(context.Background(), nil, nil) == nil {
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

// The one lane check refuses a lane nobody reviewed and a review of other
// wording, and both the precheck and the accept step ask it. A re-driven
// decision reaches the accept step without the precheck.
func TestEveryPathRefusesAnUnreviewedNotice(t *testing.T) {
	current, _ := consent.ControllerTemplateVersion(consent.TemplatePrivacyNotice)
	if err := siteLeadLaneRefusal(0); !errors.Is(err, errSiteLeadCaptureClosed) {
		t.Errorf("a lane with no reviewed version = %v, want closed", err)
	}
	if err := siteLeadLaneRefusal(current); err != nil {
		t.Errorf("a lane naming the version this build sends = %v, want open", err)
	}
	stale := &siteLeadNoticeLane{reviewed: current + 1}
	if err := siteLeadPrecheck(stale)(context.Background(), nil, nil); !errors.Is(err, errSiteLeadNoticeUnreviewed) {
		t.Errorf("the precheck on a review of other wording = %v, want unreviewed", err)
	}
	effect := siteLeadAcceptEffect(nil, nil, nil, stale)
	if err := effect(context.Background(), ids.New[ids.ApprovalKind](), []byte(`{}`), "hash"); !errors.Is(err, errSiteLeadNoticeUnreviewed) {
		t.Errorf("the accept step on a review of other wording = %v, want unreviewed before it redeems", err)
	}
}
