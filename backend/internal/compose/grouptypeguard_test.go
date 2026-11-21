// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/events"
)

const kernelEventsPath = "github.com/margince/margince/backend/internal/shared/kernel/events"

// typeGuard names where a declaring group's handler is written and the kernel
// set its first statement must test.
type typeGuard struct {
	file, recv, fn, setName string
	set                     events.TypeSet
}

func declaredTypeGuards() map[string]typeGuard {
	return map[string]typeGuard{
		"cg:ai-budget-resume":        {"aibudgettrigger.go", "", "AIBudgetResumeTrigger", "AIBudgetResumeTypes", events.AIBudgetResumeTypes},
		"cg:approval-notify":         {"approvalnotify.go", "ApprovalNotify", "HandleEvent", "ApprovalNotifyTypes", events.ApprovalNotifyTypes},
		"cg:approval-notice-retract": {"approvalnoticeretract.go", "ApprovalNoticeRetract", "HandleEvent", "ApprovalNoticeRetractTypes", events.ApprovalNoticeRetractTypes},
		"cg:audience-rescope":        {"audiencerescope.go", "AudienceRescopeGen", "HandleEvent", "AudienceRescopeTypes", events.AudienceRescopeTypes},
		"cg:vcard-ingest":            {"vcardingesttrigger.go", "VCardIngestTrigger", "HandleEvent", "VCardIngestTypes", events.VCardIngestTypes},
		"cg:contact-data":            {"contactdataenrich.go", "ContactDataEnrich", "HandleEvent", "ContactDataTypes", events.ContactDataTypes},
		"cg:overnight-agent":         {"runnerresume.go", "RunnerService", "HandleEvent", "OvernightAgentTypes", events.OvernightAgentTypes},
		"cg:commissions":             {"commissiongen.go", "CommissionGen", "HandleEvent", "CommissionsTypes", events.CommissionsTypes},
		"cg:partner-activation":      {"partneractivation.go", "PartnerActivation", "HandleEvent", "PartnerActivationTypes", events.PartnerActivationTypes},
		"cg:ai-activity":             {"../modules/aiactivity/handler.go", "Consumer", "HandleEvent", "AIActivityTypes", events.AIActivityTypes},
		"cg:scheduled-send-stop":     {"scheduledsendstop.go", "ScheduledSendStop", "HandleEvent", "ScheduledSendStopTypes", events.ScheduledSendStopTypes},
		"cg:commitment-settle":       {"commitmentsettle.go", "CommitmentSettleTrigger", "HandleEvent", "CommitmentSettleTypes", events.CommitmentSettleTypes},
		"cg:notice-case-open":        {"noticecaseopen.go", "NoticeCaseOpen", "HandleEvent", "NoticeCaseOpenTypes", events.NoticeCaseOpenTypes},
		"cg:company-auto-enrich":     {"companyautoenrich.go", "CompanyAutoEnrichTrigger", "HandleEvent", "CompanyAutoEnrichTypes", events.CompanyAutoEnrichTypes},
		"cg:contact-auto-enrich":     {"contactautoenrich.go", "ContactAutoEnrich", "HandleEvent", "ContactAutoEnrichTypes", events.ContactAutoEnrichTypes},
		"cg:cohort-promote":          {"cohortpromotegen.go", "CohortPromoteGen", "HandleEvent", "CohortPromoteTypes", events.CohortPromoteTypes},
		"cg:capture-enrich":          {"captureenrichtrigger.go", "CaptureEnrichTrigger", "HandleEvent", "CaptureEnrichTypes", events.CaptureEnrichTypes},
		"cg:stage-evidence":          {"stageevidencetrigger.go", "StageEvidenceTrigger", "HandleEvent", "StageEvidenceTypes", events.StageEvidenceTypes},
		"cg:voice-copy-recheck":      {"voicecopyrecheck.go", "VoiceCopyRecheckGen", "HandleEvent", "ActivityNarrowingTypes", events.ActivityNarrowingTypes},
		"cg:context-graph":           {"../modules/search/embedgen.go", "EmbedGen", "HandleEvent", "ContextGraphTypes", events.ContextGraphTypes},
	}
}

// A handler whose first test reads a literal or another set can act on a type
// its group does not declare. A payload-dependent nil hides that from the probe
// in grouptypes_test.go. So each handler must open with a test of its group's
// own set.
func TestEveryDeclaringHandlerStartsWithItsGroupsTypeTest(t *testing.T) {
	guards := declaredTypeGuards()
	for _, group := range events.Groups() {
		guard, listed := guards[group.Name]
		delete(guards, group.Name)
		if !group.Types.Declared() {
			continue
		}
		if !listed {
			t.Errorf("%s declares its types but has no entry in declaredTypeGuards", group.Name)
			continue
		}
		if !sameTypeSet(group.Types, guard.set) {
			t.Errorf("%s declares a set other than events.%s", group.Name, guard.setName)
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, filepath.FromSlash(guard.file), nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", guard.file, err)
		}
		if err := checkTypeGuard(file, guard.recv, guard.fn, guard.setName); err != nil {
			t.Errorf("%s (%s): %v", group.Name, guard.file, err)
		}
	}
	for name := range guards {
		t.Errorf("declaredTypeGuards names %s, which declares no types", name)
	}
}

func TestTheTypeGuardCheckRefusesALiteralOrAnotherSet(t *testing.T) {
	const header = "package p\nimport kev \"" + kernelEventsPath + "\"\ntype H struct{}\n"
	for name, body := range map[string]string{
		"a literal":         `if e.Type != "deal.stage_changed" { return nil }`,
		"another set":       `if !kev.PartnerActivationTypes.Has(e.Type) { return nil }`,
		"a later test":      `if e.Entity.ID.IsZero() || !kev.CommissionsTypes.Has(e.Type) { return nil }`,
		"another statement": `x := 1; _ = x; if !kev.CommissionsTypes.Has(e.Type) { return nil }`,
	} {
		file, err := parser.ParseFile(token.NewFileSet(), "planted.go",
			header+"func (H) HandleEvent(ctx any, e kev.Envelope) error { "+body+"; return nil }", 0)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if checkTypeGuard(file, "H", "HandleEvent", "CommissionsTypes") == nil {
			t.Errorf("a handler that starts with %s passed the type guard check", name)
		}
	}
	file, err := parser.ParseFile(token.NewFileSet(), "planted.go",
		header+"func (H) HandleEvent(ctx any, e kev.Envelope) error { if !kev.CommissionsTypes.Has(e.Type) || e.Entity.ID.IsZero() { return nil }; return nil }", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkTypeGuard(file, "H", "HandleEvent", "CommissionsTypes"); err != nil {
		t.Errorf("the guard as handlers write it was refused: %v", err)
	}
}

// sameTypeSet reports whether two sets are the one value, not two equal lists.
func sameTypeSet(a, b events.TypeSet) bool {
	return reflect.ValueOf(a).Field(0).Pointer() == reflect.ValueOf(b).Field(0).Pointer()
}

// checkTypeGuard finds the named function and requires its first statement to
// be an if whose leftmost condition is !<kernel>.<setName>.Has(<env>.Type). A
// function that returns a handler literal is checked inside that literal.
func checkTypeGuard(file *ast.File, recv, fn, setName string) error {
	alias := kernelAlias(file)
	if alias == "" {
		return fmt.Errorf("the file does not import %s", kernelEventsPath)
	}
	decl := findFunc(file, recv, fn)
	if decl == nil {
		return fmt.Errorf("no func %s.%s", recv, fn)
	}
	params, body := decl.Type.Params, decl.Body
	if ret, ok := firstStmt(body).(*ast.ReturnStmt); ok && len(ret.Results) == 1 {
		if lit, ok := ret.Results[0].(*ast.FuncLit); ok {
			params, body = lit.Type.Params, lit.Body
		}
	}
	if len(params.List) < 2 || len(params.List[1].Names) != 1 {
		return fmt.Errorf("%s does not take (ctx, env)", fn)
	}
	env := params.List[1].Names[0].Name
	ifStmt, ok := firstStmt(body).(*ast.IfStmt)
	if !ok || ifStmt.Init != nil {
		return fmt.Errorf("the first statement is not an if testing %s.%s", alias, setName)
	}
	cond := ifStmt.Cond
	for {
		or, ok := cond.(*ast.BinaryExpr)
		if !ok || or.Op != token.LOR {
			break
		}
		cond = or.X
	}
	if !readsSet(cond, alias, setName, env) {
		return fmt.Errorf("the first test is not !%s.%s.Has(%s.Type)", alias, setName, env)
	}
	return nil
}

func readsSet(cond ast.Expr, alias, setName, env string) bool {
	not, ok := cond.(*ast.UnaryExpr)
	if !ok || not.Op != token.NOT {
		return false
	}
	call, ok := not.X.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || !isSelector(call.Args[0], env, "Type") {
		return false
	}
	has, ok := call.Fun.(*ast.SelectorExpr)
	return ok && has.Sel.Name == "Has" && isSelector(has.X, alias, setName)
}

func isSelector(expr ast.Expr, x, sel string) bool {
	s, ok := expr.(*ast.SelectorExpr)
	if !ok || s.Sel.Name != sel {
		return false
	}
	id, ok := s.X.(*ast.Ident)
	return ok && id.Name == x
}

func firstStmt(body *ast.BlockStmt) ast.Stmt {
	if body == nil || len(body.List) == 0 {
		return nil
	}
	return body.List[0]
}

func kernelAlias(file *ast.File) string {
	for _, imp := range file.Imports {
		if path, err := strconv.Unquote(imp.Path.Value); err != nil || path != kernelEventsPath {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		return "events"
	}
	return ""
}

func findFunc(file *ast.File, recv, fn string) *ast.FuncDecl {
	for _, d := range file.Decls {
		decl, ok := d.(*ast.FuncDecl)
		if !ok || decl.Name.Name != fn || receiverName(decl) != recv {
			continue
		}
		return decl
	}
	return nil
}

func receiverName(decl *ast.FuncDecl) string {
	if decl.Recv == nil || len(decl.Recv.List) == 0 {
		return ""
	}
	typ := decl.Recv.List[0].Type
	if star, ok := typ.(*ast.StarExpr); ok {
		typ = star.X
	}
	if id, ok := typ.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}
