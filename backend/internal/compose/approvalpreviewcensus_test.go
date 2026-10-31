// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestEveryGovernedKindHasReviewContextOrIsNotCurrentlyStageable(t *testing.T) {
	registry := NewRegistry(nil, SendPath{})
	active := map[string]bool{}
	for _, spec := range registry.Specs() {
		if registry.Stageable(spec.Name) {
			active[spec.Name] = true
		}
	}
	for _, kind := range automation.StageableKinds() {
		active[kind] = true
	}
	for _, kind := range approvalsServiceWithEffects(nil).EffectKinds() {
		active[kind] = true
	}
	for _, policy := range agentPolicies {
		if policy.Access == accessTool && policy.Tier != tierAutoExecute {
			active[policy.Tool] = true
		}
	}
	governed := approvals.StageableKinds()
	if len(governed) == 0 || len(active) == 0 {
		t.Fatal("the approval producer census is empty")
	}
	reader := approvalPreviewReader{}
	for _, kind := range governed {
		if inactiveApprovalKind(kind) {
			if active[kind] {
				t.Errorf("%s became stageable without a display", kind)
			}
			delete(active, kind)
			continue
		}
		assertApprovalContextCovered(t, reader, kind)
		delete(active, kind)
	}
	for kind := range active {
		t.Errorf("active producer %s is missing from the governed approval catalog", kind)
	}
}

func inactiveApprovalKind(kind string) bool {
	// The first two execute without staging; the two retired kinds remain readable in history.
	switch kind {
	case "archive_tag", "change_record_parts", "transcript_proposal", "communication_review":
		return true
	}
	return false
}

func assertApprovalContextCovered(t *testing.T, reader approvalPreviewReader, kind string) {
	t.Helper()
	preview, err := reader.read(context.Background(), crmcontracts.Approval{Kind: kind, Status: crmcontracts.ApprovalStatusPending})
	if err != nil {
		t.Fatalf("%s missing-context projection: %v", kind, err)
	}
	if hasTypedProposalDisplay(kind) {
		if preview != nil {
			t.Errorf("%s replaced its established typed display", kind)
		}
		return
	}
	if preview == nil {
		t.Fatalf("%s falls back to raw arguments", kind)
	}
	if preview.Conflicts != nil {
		for _, conflict := range *preview.Conflicts {
			if conflict.Kind == "unsupported_approval_kind" {
				t.Errorf("%s has no display projection", kind)
			}
		}
	}
	if err := approvals.PreviewRefusal(preview); err == nil {
		t.Errorf("%s permits approval without required context", kind)
	}
}

// An extension's staged kind is its tool name, registered at boot from the
// served tools; the census walks that same derivation.
func TestEveryStageableExtensionVerbHasReviewContext(t *testing.T) {
	kinds := extensionStagingKinds(confirmFirstUnit(t))
	if len(kinds) == 0 {
		t.Fatal("the confirm-first unit derived no staged kind")
	}
	if err := approvals.RegisterExtensionKinds(kinds); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := approvals.RegisterExtensionKinds(nil); err != nil {
			t.Errorf("clearing the registered kinds: %v", err)
		}
	})
	for _, kind := range kinds {
		assertApprovalContextCovered(t, approvalPreviewReader{}, kind.Verb)
	}
}

func TestAnUnknownApprovalKindRequiresSupportedContext(t *testing.T) {
	preview, err := (approvalPreviewReader{}).read(context.Background(), crmcontracts.Approval{Kind: "new_unsupported_operation", Status: crmcontracts.ApprovalStatusPending})
	if err != nil {
		t.Fatal(err)
	}
	if preview == nil || approvals.PreviewRefusal(preview) == nil {
		t.Fatal("unknown effect can be approved without context")
	}
}

// Every kind's preview first asks who runs its effect. An agent that can no
// longer call cannot spend its approval, so no kind may show it as decidable.
func TestEveryApprovalKindIsPreviewedAsThePrincipalThatRunsIt(t *testing.T) {
	kinds := approvals.StageableKinds()
	extensions := extensionStagingKinds(confirmFirstUnit(t))
	if len(kinds) == 0 || len(extensions) == 0 {
		t.Fatal("the approval kind census is empty")
	}
	if err := approvals.RegisterExtensionKinds(extensions); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := approvals.RegisterExtensionKinds(nil); err != nil {
			t.Errorf("clearing the registered kinds: %v", err)
		}
	})
	for _, extension := range extensions {
		kinds = append(kinds, extension.Verb)
	}
	for _, kind := range kinds {
		asked := false
		revoked := func(ctx context.Context) (context.Context, bool, error) {
			asked = true
			return ctx, true, apperrors.ErrPermissionDenied
		}
		preview, err := (approvalPreviewReader{}).readAs(context.Background(), crmcontracts.Approval{Kind: kind, Status: crmcontracts.ApprovalStatusPending}, revoked)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if !asked {
			t.Errorf("%s is previewed without asking who runs its effect", kind)
		}
		if approvals.PreviewRefusal(preview) == nil {
			t.Errorf("%s can be approved although its agent can no longer call", kind)
		}
	}
}

// A kind cannot be exercised through the service without a database, so this
// census reads the dispatch source. Only readAs builds a preview, and nothing
// returns before it asks who runs the effect. Only read hands it the approvals
// module's answer, and the service is wired to read.
func TestOnlyTheDispatchBuildsAnApprovalPreview(t *testing.T) {
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	wired, built := 0, 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				if call, ok := node.(*ast.CallExpr); ok && wiresTheDispatch(call) {
					wired++
				}
				selector, ok := node.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				switch selector.Sel.Name {
				case "buildPreview":
					built++
					if fn.Name.Name != "readAs" {
						t.Errorf("%s: %s builds a preview outside the dispatch", fset.Position(selector.Pos()), fn.Name.Name)
					}
				case "EffectContext":
					if fn.Name.Name != "read" {
						t.Errorf("%s: %s asks who runs the effect outside the dispatch", fset.Position(selector.Pos()), fn.Name.Name)
					}
				}
				return true
			})
			if fn.Name.Name == "readAs" {
				assertNothingReturnsBeforeTheEffect(t, fset, fn)
			}
		}
	}
	if built != 1 {
		t.Errorf("the dispatch builds a preview %d times, want once in readAs", built)
	}
	if wired != 1 {
		t.Errorf("the approvals service is wired to a preview %d times, want once through read", wired)
	}
}

// wiresTheDispatch answers whether call hands read to WithPreview.
func wiresTheDispatch(call *ast.CallExpr) bool {
	fun, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || fun.Sel.Name != "WithPreview" || len(call.Args) != 1 {
		return false
	}
	arg, ok := call.Args[0].(*ast.SelectorExpr)
	return ok && arg.Sel.Name == "read"
}

// assertNothingReturnsBeforeTheEffect holds that readAs answers no kind before
// it asks who runs the effect, except a terminal approval, which has no preview.
func assertNothingReturnsBeforeTheEffect(t *testing.T, fset *token.FileSet, fn *ast.FuncDecl) {
	t.Helper()
	asked := token.NoPos
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok && asked == token.NoPos {
			if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "effectOf" {
				asked = call.Pos()
			}
		}
		return true
	})
	if asked == token.NoPos {
		t.Fatal("readAs never asks who runs the effect")
	}
	early := 0
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if ret, ok := node.(*ast.ReturnStmt); ok && ret.Pos() < asked {
			early++
		}
		return true
	})
	if early > 1 {
		t.Errorf("%s: readAs answers %d times before asking who runs the effect, want only the terminal approval", fset.Position(fn.Pos()), early)
	}
}
