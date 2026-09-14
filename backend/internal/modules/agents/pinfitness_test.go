// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// A tool that STAGES a target version APPLIES one (review-loop rule 2).
//
// The staging half already holds itself together: approvals.resolveTargetVersion
// is the one place a pin is taken, and it takes one for every staged call whose
// target has a version column and whose kind is not declared context-only or
// unpinned. What nothing held is the other end — that the write the approval
// releases is actually conditioned on the pin the human approved.
//
// Why that gap is not cosmetic. Redemption commits its OWN transaction and the
// handler then opens a fresh one, so the skew check inside redemption proves the
// row was at the approved version when the approval was CONSUMED, not when the
// effect lands — and the agent controls both sides of that window, since its own
// 🟢 update_record can commit inside it. The pin is what moves the version
// compare into the transaction that mutates.
//
// The REST door gets this for free: compose forwards the released pin as the
// request's own If-Match, so every operation behind that door is carried,
// whether or not anyone remembered. The MCP door carries it per tool, through
// pinForWrite — which is a call site somebody has to remember, and update_record
// and disqualify_lead were both missing it. One credential, two doors, different
// guarantees, which is the asymmetry ADR-0055 exists to close.
//
// So the corpus is the registry rather than a list, and the obligation is
// checked in source rather than asserted per tool: a new stageable write is
// judged the day it registers, not the day somebody remembers to add it here.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// pinExemptWrites are the stageable tools whose write has NO version to
// condition, each saying which structural reason it is.
//
// Not a duplicate of approvals' own contextTargetKinds/unpinnedKinds, which a
// reader might reasonably suspect. Those are keyed on staging KIND and describe
// automation proposals (site_lead, deal_follow_up, capture_counterparty); these
// are keyed on the TOOL TYPE and describe MCP write shapes. The two sets do not
// intersect, and a module never imports a sibling, so this side could not read
// that one even where they agreed.
//
// Each entry is structural rather than pending: a call that creates a row has no
// prior version, and one pin cannot condition many rows. A third KIND of reason
// appearing here is a finding — the list is meant to stay short and each line to
// say why the rule does not reach.
var pinExemptWrites = gatekit.Waive(map[string]string{
	"createRecord":         "a create has no prior row, so there is no version to condition on. approvals.resolveTargetVersion answers unpinned for it on exactly the same ground — the target id is zero — so nothing is staged that this write fails to apply",
	"sendMessageTool":      "the effect is a NEW activity filed against the target, not a write to it. A pin on the conversation would condition the reply on the thread not having moved, which is not what the human judged: they judged the words being sent",
	"sendEmailTool":        "the effect is a NEW activity, on the ground sendMessageTool states. The target is what the message is ABOUT, and the version of a record nobody is editing binds nothing the approval rests on",
	"sendAccountEmailTool": "the effect is a NEW activity and the call anchors on no row at all — its links carry the target. There is no operand row whose version could condition the send",
	"bookMeetingTool":      "a booking creates its own row and anchors on no existing one; the links name what it is about. Nothing it writes has a prior version",
	"mergeRecords":         "the merge conditions itself on both endpoints INSIDE its own transaction, under the locks it takes on the pair (contacts/merge.go). A pin here would be a third answer to a question the write already asks race-free, and only about the survivor — the source is the half a caller-supplied pin could never cover",
	"relinkThread":         "a thread is MANY activity rows reached by key, and one pin cannot condition many rows. Its sibling relinkActivity takes one because it names exactly one activity; the batch shape is what makes the pin unrepresentable rather than forgotten, and relinkbatch.go re-checks each row under its own lock",
	"relinkActivities":     "the batch form of relinkThread's reason: the call names a LIST of activities and a single version could only ever condition one of them. The per-row check under the write's own lock is what holds here, and it holds for every row rather than for one",
})

// pinDeferredWrites are stageable writes that SHOULD apply a pin and do not.
//
// A real verdict rather than a quiet exclusion: a bare exemption reads as
// "handled", this reads as "decided, and not yet done", and the count below
// keeps the hole visible. Every entry names its issue, which is a convention
// rather than a checked rule for the reason the edge-reader census gives about
// its own — a parallel map policing this one is how the two diverge.
//
// This set is EMPTY when nothing is pending, which is the state to get it back
// to. It stays declared rather than deleted, because the next write that cannot
// carry a pin today needs somewhere honest to go.
var pinDeferredWrites = gatekit.Waive(map[string]string{
	"promoteLead": "stages a pinnable lead version and applies none: LeadPromoter cannot carry one, exactly as LeadDisqualifier could not. The promotion mints a contact from lead fields a concurrent edit may have changed since the human approved. Tracked in issue 5021 — the cost of leaving it is that an approved promotion can read content nobody released",
	"demoteLead":  "stages a pinnable lead version and applies none, on the same seam shape as promoteLead. The reversal unwinds a promotion the approval may no longer be describing. Tracked in issue 5021 — same cost, in the opposite direction",
})

// pinnedWriteFloor is set below the live count of tools that must apply a pin.
// It catches the reflection or the registry walk going to zero, which reports
// PASS having judged nothing.
const pinnedWriteFloor = 4

// TestEveryStageableWriteAppliesTheVersionItStages walks the registry's
// stageable tools and requires each one's Handle to reach pinForWrite.
//
// Per TYPE and via reflection rather than by tool name, because the name is the
// wire's and the body is the type's: a rename on either side would otherwise
// silently drop a tool out of the walk, and a walk that judges fewer tools than
// it thinks reports PASS exactly like a clean tree.
func TestEveryStageableWriteAppliesTheVersionItStages(t *testing.T) {
	t.Parallel()
	defer pinExemptWrites.AssertAllMatched(t)
	defer pinDeferredWrites.AssertAllMatched(t)

	reaching := handlesReachingPinForWrite(t)
	registry := NewRegistry(&recordingApprovals{}, nil)
	RegisterCoreTools(registry, elsewhereProvider{}, fixedStages{semantic: "won"}, nil, noConflicts{}, nil, nil)
	RegisterCommsTools(registry, &recordingComms{}, elsewhereProvider{})
	RegisterLifecycleTools(registry, elsewhereProvider{}, nil, inertLifecycle{}, inertLifecycle{}, inertLifecycle{})

	pinned := 0
	for name, tool := range registry.tools {
		if _, isStageable := tool.(stageableTool); !isStageable {
			continue
		}
		typeName := toolTypeName(tool)
		verdict := pinVerdictFor(t, typeName)
		reaches := reaching[typeName]
		switch {
		case reaches && verdict != "":
			// A satisfied write that ALSO carries a verdict is how this census
			// silently stops counting down: a deferred write that later gets
			// its pin keeps matching for ever, and the hole reads as open when
			// it is closed.
			t.Errorf("%s (%s) applies a pin AND carries a %s verdict — remove the verdict, "+
				"it describes code that now carries the rule", name, typeName, verdict)
		case reaches, verdict != "":
		default:
			t.Errorf("%s (%s) can stage an approval but its Handle never reaches pinForWrite.\n"+
				"  A staged call whose target has a version column gets one pinned at staging "+
				"(approvals.resolveTargetVersion), and the write that releases it must be conditioned "+
				"on that version — redemption commits its own transaction, so its skew check proves "+
				"the row was right when the approval was CONSUMED, not when this write lands.\n"+
				"  Route the write's version through pinForWrite(ctx, <the caller's if_version, or nil>), "+
				"or declare the type in pinExemptWrites with the reason its write has no version to "+
				"condition.", name, typeName)
		}
		if reaches {
			pinned++
		}
	}
	t.Logf("stageable writes: %d apply a pin, %d structurally exempt, %d DEFERRED (still unpinned)",
		pinned, len(pinExemptWrites.Subjects()), len(pinDeferredWrites.Subjects()))
	if pinned < pinnedWriteFloor {
		t.Fatalf("only %d stageable writes were seen to apply a pin, below the %d floor — either the "+
			"registry walk or the source census has stopped seeing this package, and both report PASS "+
			"while judging nothing", pinned, pinnedWriteFloor)
	}
}

// pinVerdictFor names the declaration a type sits in, and refuses one sitting in
// two: the verdict IS the declaration, so a type with two of them has none.
func pinVerdictFor(t *testing.T, typeName string) string {
	t.Helper()
	var found []string
	for _, set := range []struct {
		name    string
		waivers *gatekit.Waivers[string]
	}{
		{"structural", pinExemptWrites},
		{"deferred", pinDeferredWrites},
	} {
		if set.waivers.Waived(t, typeName) {
			found = append(found, set.name)
		}
	}
	if len(found) > 1 {
		t.Errorf("%s carries %s verdicts at once: which declaration a write sits in IS its verdict, "+
			"so two of them is none", typeName, strings.Join(found, " and "))
		return ""
	}
	if len(found) == 1 {
		return found[0]
	}
	return ""
}

// toolTypeName is the tool's own Go type, through a pointer receiver.
//
//craft:ignore naked-any the registry holds tools by the interface each satisfies, and this asks what CONCRETE type one is — a constrained parameter would name the very thing being read
func toolTypeName(tool any) string {
	t := reflect.TypeOf(tool)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Name()
}

// handlesReachingPinForWrite answers, per receiver type, whether its Handle
// reaches pinForWrite — in its own body, or in a package-local function that
// body calls.
//
// One level of following, and the reason is the one the job-actor census gives
// about its own follow: a helper is what a write's pin resolution naturally
// lives in, and a rule that read only the entry point would produce exemptions
// for correct code. A rule that produces exemptions for correct code is one
// whose exemption list stops meaning anything.
func handlesReachingPinForWrite(t *testing.T) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	// Bodies by declaration name: methods under "Type.Method", plain functions
	// under their bare name — the two vocabularies cannot collide, because a
	// bare-name lookup can never reach a key holding a dot.
	bodies := map[string]*ast.FuncDecl{}
	handles := map[string]*ast.FuncDecl{}
	for _, path := range agentsSourceFiles(t) {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s for the pin census: %v", path, err)
		}
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Body == nil {
				continue
			}
			if fn.Recv == nil {
				bodies[fn.Name.Name] = fn
				continue
			}
			recv := receiverTypeOf(fn)
			bodies[recv+"."+fn.Name.Name] = fn
			if fn.Name.Name == "Handle" {
				handles[recv] = fn
			}
		}
	}
	if len(handles) == 0 {
		t.Fatal("the census indexed no Handle methods at all — it is reading a tree it does not " +
			"recognise, and an empty index judges every tool as failing rather than as unseen")
	}
	reaching := map[string]bool{}
	for recv, fn := range handles {
		reaching[recv] = reachesPinForWrite(fn, recv, bodies)
	}
	return reaching
}

// reachesPinForWrite is the body's own call to pinForWrite, or one made
// anywhere down the chain of package functions it calls.
//
// TRANSITIVE, not one level. update_record is why: its Handle branches to
// t.apply, which calls t.applyRecord, which is where the pin belongs — the
// entry point is a dispatcher and the write is two hops down. A census that
// stopped at one hop would have reported the tool this ticket is about as
// still broken after it was fixed, which is the shape that gets a gate
// deleted.
//
// seen is the cycle guard and the memo both; a name already on the stack
// answers false, because a cycle that reached the pin reported it on the way in.
func reachesPinForWrite(fn *ast.FuncDecl, recv string, bodies map[string]*ast.FuncDecl) bool {
	return reachesFrom(fn, recv, bodies, map[string]bool{})
}

func reachesFrom(fn *ast.FuncDecl, recv string, bodies map[string]*ast.FuncDecl, seen map[string]bool) bool {
	if callsPinForWrite(fn.Body) {
		return true
	}
	for _, name := range calledDeclNames(fn.Body, recv, receiverNameOf(fn)) {
		if seen[name] {
			continue
		}
		seen[name] = true
		if called, known := bodies[name]; known && reachesFrom(called, recv, bodies, seen) {
			return true
		}
	}
	return false
}

func callsPinForWrite(body ast.Node) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if ident, isIdent := n.(*ast.Ident); isIdent && ident.Name == "pinForWrite" {
			found = true
		}
		return !found
	})
	return found
}

// calledDeclNames collects the calls in the body this package can resolve by
// name: a bare identifier, and a selector on the body's OWN receiver — matched against
// the receiver's actual variable name, since a census that assumed one spelling
// would follow nothing in every method that chose another, and report those
// tools as carrying no pin.
//
// A selector on anything else names a method on a type this census cannot
// resolve without type information, and following it by name would answer a
// different question.
func calledDeclNames(body ast.Node, recv, recvVar string) []string {
	var names []string
	ast.Inspect(body, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			names = append(names, fun.Name)
		case *ast.SelectorExpr:
			if base, isIdent := fun.X.(*ast.Ident); isIdent && recvVar != "" && base.Name == recvVar {
				names = append(names, recv+"."+fun.Sel.Name)
			}
		}
		return true
	})
	return names
}

// receiverNameOf is the receiver's variable name, empty for an unnamed one.
func receiverNameOf(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 || len(fn.Recv.List[0].Names) == 0 {
		return ""
	}
	return fn.Recv.List[0].Names[0].Name
}

func receiverTypeOf(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	expr := fn.Recv.List[0].Type
	if star, isStar := expr.(*ast.StarExpr); isStar {
		expr = star.X
	}
	if ident, isIdent := expr.(*ast.Ident); isIdent {
		return ident.Name
	}
	return ""
}

// agentsSourceFiles is this package's own non-test, non-generated sources.
func agentsSourceFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the agents package for the pin census: %v", err)
	}
	var paths []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_gen.go") {
			continue
		}
		paths = append(paths, name)
	}
	slices.Sort(paths)
	return paths
}
