// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The subscriber acks every event type a consumer group does not declare
// before the group's handler runs. These gates hold each declaring handler to
// its group's kernel TypeSet. One probes it with every left-out type. The
// other reads that its first statement tests that set.

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/aiactivity"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
	"github.com/margince/margince/backend/pkg/extension"
)

// unusedRuns is a provider run service that panics when called, so a handler
// that passes its nil check and goes on to work is seen.
type unusedRuns struct{ provider.RunService }

// declaredTypeHandlers are the handlers of every group that declares its types.
// They are built with no dependencies, so a handler that goes past its type
// test reaches a nil store or pool and fails.
func declaredTypeHandlers() map[string]func(context.Context, events.Envelope) error {
	return map[string]func(context.Context, events.Envelope) error{
		"cg:ai-budget-resume":        compose.AIBudgetResumeTrigger(nil),
		"cg:approval-notify":         (&compose.ApprovalNotify{}).HandleEvent,
		"cg:approval-notice-retract": (&compose.ApprovalNoticeRetract{}).HandleEvent,
		"cg:audience-rescope":        (&compose.AudienceRescopeGen{}).HandleEvent,
		"cg:vcard-ingest":            (&compose.VCardIngestTrigger{}).HandleEvent,
		"cg:contact-data":            compose.NewContactDataEnrich(nil, unusedRuns{}, nil).HandleEvent,
		"cg:overnight-agent":         (&compose.RunnerService{}).HandleEvent,
		"cg:commissions":             (&compose.CommissionGen{}).HandleEvent,
		"cg:partner-activation":      (&compose.PartnerActivation{}).HandleEvent,
		"cg:ai-activity":             (&aiactivity.Consumer{}).HandleEvent,
		"cg:scheduled-send-stop":     (&compose.ScheduledSendStop{}).HandleEvent,
		"cg:commitment-settle":       (&compose.CommitmentSettleTrigger{}).HandleEvent,
		"cg:notice-case-open":        (&compose.NoticeCaseOpen{}).HandleEvent,
		"cg:company-auto-enrich":     (&compose.CompanyAutoEnrichTrigger{}).HandleEvent,
		"cg:contact-auto-enrich":     (&compose.ContactAutoEnrich{}).HandleEvent,
		"cg:cohort-promote":          (&compose.CohortPromoteGen{}).HandleEvent,
		"cg:capture-enrich":          (&compose.CaptureEnrichTrigger{}).HandleEvent,
		"cg:stage-evidence":          (&compose.StageEvidenceTrigger{}).HandleEvent,
		"cg:voice-copy-recheck":      (&compose.VoiceCopyRecheckGen{}).HandleEvent,
		"cg:context-graph":           (&search.EmbedGen{}).HandleEvent,
	}
}

// The subscriber acks every type a group does not declare before its handler
// runs. A handler that acts on a type its group leaves out loses that event
// without an error. So each handler is fed every other type on its streams and
// must ignore it.
func TestEveryHandlerIgnoresTheTypesItsGroupLeavesOut(t *testing.T) {
	t.Parallel()
	handlers := declaredTypeHandlers()
	for _, group := range events.Groups() {
		handler, listed := handlers[group.Name]
		if !group.Types.Declared() {
			if listed {
				t.Errorf("%s is listed here but declares no types; drop it from declaredTypeHandlers", group.Name)
			}
			continue
		}
		delete(handlers, group.Name)
		if !listed {
			t.Errorf("%s declares its types but has no handler in declaredTypeHandlers, so nothing checks them", group.Name)
			continue
		}
		for _, eventType := range events.Types() {
			stream, err := events.StreamFor(eventType)
			if err != nil {
				t.Fatalf("routing %s: %v", eventType, err)
			}
			if group.Types.Has(eventType) || !slices.Contains(group.Streams, stream) {
				continue
			}
			for _, entity := range entityTypesFor(eventType) {
				if err := ignores(t, handler, eventType, entity); err != nil {
					t.Errorf("%s acts on %s (entity %s), which its group does not declare: %v", group.Name, eventType, entity, err)
				}
			}
		}
	}
	for name := range handlers {
		t.Errorf("declaredTypeHandlers names %s, which is not a consumer group", name)
	}
}

// An extension group declares the types its subscription names, so the
// subscriber acks the rest of the streams those types ride.
func TestAnExtensionGroupDeclaresItsSubscriptionsTypes(t *testing.T) {
	t.Parallel()
	types := []string{"activity.archived", "activity.captured", "ext_notes.note_added"}
	group, err := compose.ComposedSubscription{
		Unit: "notes", Version: "1.0.0", Sub: extension.Subscription{Name: "listener", Events: types},
	}.Group()
	if err != nil {
		t.Fatal(err)
	}
	if got := group.Types.List(); !slices.Equal(got, types) {
		t.Errorf("the extension group declares %v, want %v", got, types)
	}
}

// entityTypesFor is the type's own entity and every record type a family
// stream event can name.
func entityTypesFor(eventType string) []string {
	out := []string{"contact", "company", "deal", "activity", "lead", "approval"}
	if entity, _, err := events.SplitType(eventType); err == nil && !slices.Contains(out, entity) {
		out = append(out, entity)
	}
	return out
}

// ignores runs handler on one event and reports whether it did anything.
func ignores(t *testing.T, handler func(context.Context, events.Envelope) error, eventType, entity string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panicked: %v", r)
		}
	}()
	return handler(context.Background(), events.Envelope{
		EventID: ids.NewV7(), Type: eventType, OccurredAt: clocktest.Now(t),
		Actor:   events.Actor{Type: "human", ID: "human:" + ids.NewV7().String()},
		Entity:  events.EntityRef{Type: entity, ID: ids.NewV7()},
		Payload: json.RawMessage(`{}`),
	})
}

const kernelEventsImport = "github.com/margince/margince/backend/internal/shared/kernel/events"

// typeGuard names where a declaring group's handler is written and the kernel
// set its first statement must test.
type typeGuard struct {
	file, recv, fn, setName string
	set                     events.TypeSet
}

func declaredTypeGuards() map[string]typeGuard {
	return map[string]typeGuard{
		"cg:ai-budget-resume":        {"compose/aibudgettrigger.go", "", "AIBudgetResumeTrigger", "AIBudgetResumeTypes", events.AIBudgetResumeTypes},
		"cg:approval-notify":         {"compose/approvalnotify.go", "ApprovalNotify", "HandleEvent", "ApprovalNotifyTypes", events.ApprovalNotifyTypes},
		"cg:approval-notice-retract": {"compose/approvalnoticeretract.go", "ApprovalNoticeRetract", "HandleEvent", "ApprovalNoticeRetractTypes", events.ApprovalNoticeRetractTypes},
		"cg:audience-rescope":        {"compose/audiencerescope.go", "AudienceRescopeGen", "HandleEvent", "AudienceRescopeTypes", events.AudienceRescopeTypes},
		"cg:vcard-ingest":            {"compose/vcardingesttrigger.go", "VCardIngestTrigger", "HandleEvent", "VCardIngestTypes", events.VCardIngestTypes},
		"cg:contact-data":            {"compose/contactdataenrich.go", "ContactDataEnrich", "HandleEvent", "ContactDataTypes", events.ContactDataTypes},
		"cg:overnight-agent":         {"compose/runnerresume.go", "RunnerService", "HandleEvent", "OvernightAgentTypes", events.OvernightAgentTypes},
		"cg:commissions":             {"compose/commissiongen.go", "CommissionGen", "HandleEvent", "CommissionsTypes", events.CommissionsTypes},
		"cg:partner-activation":      {"compose/partneractivation.go", "PartnerActivation", "HandleEvent", "PartnerActivationTypes", events.PartnerActivationTypes},
		"cg:ai-activity":             {"modules/aiactivity/handler.go", "Consumer", "HandleEvent", "AIActivityTypes", events.AIActivityTypes},
		"cg:scheduled-send-stop":     {"compose/scheduledsendstop.go", "ScheduledSendStop", "HandleEvent", "ScheduledSendStopTypes", events.ScheduledSendStopTypes},
		"cg:commitment-settle":       {"compose/commitmentsettle.go", "CommitmentSettleTrigger", "HandleEvent", "CommitmentSettleTypes", events.CommitmentSettleTypes},
		"cg:notice-case-open":        {"compose/noticecaseopen.go", "NoticeCaseOpen", "HandleEvent", "NoticeCaseOpenTypes", events.NoticeCaseOpenTypes},
		"cg:company-auto-enrich":     {"compose/companyautoenrich.go", "CompanyAutoEnrichTrigger", "HandleEvent", "CompanyAutoEnrichTypes", events.CompanyAutoEnrichTypes},
		"cg:contact-auto-enrich":     {"compose/contactautoenrich.go", "ContactAutoEnrich", "HandleEvent", "ContactAutoEnrichTypes", events.ContactAutoEnrichTypes},
		"cg:cohort-promote":          {"compose/cohortpromotegen.go", "CohortPromoteGen", "HandleEvent", "CohortPromoteTypes", events.CohortPromoteTypes},
		"cg:capture-enrich":          {"compose/captureenrichtrigger.go", "CaptureEnrichTrigger", "HandleEvent", "CaptureEnrichTypes", events.CaptureEnrichTypes},
		"cg:stage-evidence":          {"compose/stageevidencetrigger.go", "StageEvidenceTrigger", "HandleEvent", "StageEvidenceTypes", events.StageEvidenceTypes},
		"cg:voice-copy-recheck":      {"compose/voicecopyrecheck.go", "VoiceCopyRecheckGen", "HandleEvent", "ActivityNarrowingTypes", events.ActivityNarrowingTypes},
		"cg:context-graph":           {"modules/search/embedgen.go", "EmbedGen", "HandleEvent", "ContextGraphTypes", events.ContextGraphTypes},
	}
}

// A handler whose first test reads a literal or another set can act on a type
// its group does not declare. A payload-dependent nil hides that from the probe
// in TestEveryHandlerIgnoresTheTypesItsGroupLeavesOut. So each handler must open with a test of its group's
// own set.
func TestEveryDeclaringHandlerStartsWithItsGroupsTypeTest(t *testing.T) {
	t.Parallel()
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
		file, err := parser.ParseFile(fset, filepath.Join(repoRoot, "backend", "internal", filepath.FromSlash(guard.file)), nil, 0)
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
	t.Parallel()
	const header = "package p\nimport kev \"" + kernelEventsImport + "\"\ntype H struct{}\n"
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
		return fmt.Errorf("the file does not import %s", kernelEventsImport)
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
	if !ok || len(call.Args) != 1 || !guardSelector(call.Args[0], env, "Type") {
		return false
	}
	has, ok := call.Fun.(*ast.SelectorExpr)
	return ok && has.Sel.Name == "Has" && guardSelector(has.X, alias, setName)
}

func guardSelector(expr ast.Expr, x, sel string) bool {
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
		if path, err := strconv.Unquote(imp.Path.Value); err != nil || path != kernelEventsImport {
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
		if !ok || decl.Name.Name != fn || guardReceiver(decl) != recv {
			continue
		}
		return decl
	}
	return nil
}

func guardReceiver(decl *ast.FuncDecl) string {
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
