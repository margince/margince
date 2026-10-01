// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A handler waiting on a model answers a lane that did not answer as the lane
// being down (modelfailure.Write), never as an opaque 500. The set it binds is
// DERIVED: every handler in this package, or in any package beneath it, whose
// calls can reach a model call. The packages are the go command's listing of
// this tree, so a new subpackage is held the day it is added.
//
// "Can reach" is followed through the handler's own package's functions and
// methods, through an interface method to every type there that implements it,
// and out of the package at the first call that takes a model request, takes a
// value able to make one, or is a method of a type holding one.

import (
	"fmt"
	"go/ast"
	"go/types"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// modelHandlersThatDegrade are the functions that catch a model call's error
// and deliberately answer without the model, so the failure never reaches
// modelfailure.Write. An entry names the function that drops the error — the
// handler when it drops it inline, the helper when a helper does — so the two
// shapes need the same entry and one helper several handlers share needs one.
var modelHandlersThatDegrade = gatekit.Waive(map[string]string{
	"(compose.Server).RegenerateOffer":               "the mechanical revision is minted first and served; a drafting failure is logged and the caller keeps that revision",
	"compose.AnswerCorpus":                           "a failed lane serves the retrieved passages with their citations as unreviewed, never as answered, and logs which lane failed",
	"(*compose.onboardingCompanyAssistant).converse": "a clicked option was proved against the read before any model was asked, so it is recorded and confirmed in a server-written sentence; with no option clicked the error is returned",
	"(compose/briefs.briefL2Ranker).reorder":         "the model re-rank is advisory over the deterministic composite order, which the brief keeps when the lane fails, with a warning logged",
	"compose/company360.writeIntroRequest":           "the template floor states every fact of the ask honestly, so the reader gets a sendable message and generated_by says the floor wrote it",
	"compose/companybrief.Answer":                    "the deterministic answer is the declared on_budget_exhausted degrade; generated_by tells the reader the floor answered",
	"compose/companybrief.Write":                     "the deterministic card is the declared on_budget_exhausted degrade; generated_by tells the reader the floor wrote it",
	"compose/companydossier.WriteDossier":            "the floor dossier is the declared degrade, labelled as the floor's, and the lane-failed flag stops it overwriting a written dossier",
	"compose/companydossier.WriteGrowthFit":          "the floor's abstention is the declared degrade, labelled as the floor's, and the lane-failed flag stops it overwriting a real assessment",
	"compose/contactbrief.Write":                     "the deterministic card is the declared on_budget_exhausted degrade; generated_by tells the reader the floor wrote it",
	"(*compose/dealstatus.Service).write":            "the deterministic card is the declared degrade, and a warning names the lane that failed so the fallback is not silent",
	"compose/meetingbrief.Write":                     "the floor brief is the declared on_budget_exhausted degrade; generated_by tells the reader the floor wrote it",
	"compose/meetingbrief.WritePlan":                 "the floor plan is the declared on_budget_exhausted degrade; generated_by tells the reader the floor wrote it",
	"compose/network.writeIntroNote":                 "the template floor states every fact of the introduction honestly, and generated_by says the floor wrote it",
})

func TestEveryHandlerReachingAModelAnswersThroughModelFailure(t *testing.T) {
	got := modelFailureCensus(t, typeCheckedComposeTree(t))
	for _, finding := range got.findings {
		t.Error(finding)
	}
	modelHandlersThatDegrade.AssertAllMatched(t)
	// The onboarding assistant, the cold start, the scrape, the site read and
	// the corpus ask all reach a model in this package, and the briefs, the
	// drafts and the dossier each in their own; fewer means the scan lost its way.
	if got.reaching < 5 || got.packages < 10 {
		t.Fatalf("found %d handlers in %d packages reaching a model — the census is reading the wrong files",
			got.reaching, got.packages)
	}
}

// modelFailureFindings is what the census found across the packages it read.
type modelFailureFindings struct {
	findings           []string
	reaching, packages int
}

// modelFailureCensus names every handler, across checked, that reaches a model
// and does not answer each model-reaching call's error through
// modelfailure.Write, unless it is waived.
func modelFailureCensus(t *testing.T, checked []*typeCheckedSources) modelFailureFindings {
	t.Helper()
	var got modelFailureFindings
	for _, pkg := range checked {
		before := got.reaching
		for _, unit := range modelHandlerUnits(pkg) {
			if !unit.reachesModel {
				continue
			}
			got.reaching++
			if unit.modelCalls == 0 && modelHandlersThatDegrade.Waived(t, unit.name) {
				continue
			}
			unit.unanswered = slices.DeleteFunc(unit.unanswered, func(drop modelDropSite) bool {
				return modelHandlersThatDegrade.Waived(t, drop.owner)
			})
			if !unit.answered() {
				got.findings = append(got.findings, unit.finding())
			}
		}
		if got.reaching > before {
			got.packages++
		}
	}
	return got
}

// A subpackage is read like the root: a handler planted in one, shaped the way
// the subpackages are — a transport over a service holding the model lane — is
// named by the census, and the same handler answering through modelfailure is
// not.
func TestTheModelFailureCensusNamesASubpackageHandler(t *testing.T) {
	const source = `package planted
import (
	"net/http"
	"github.com/margince/margince/backend/internal/compose/modelfailure"
	"github.com/margince/margince/backend/internal/platform/httperr"
	model "` + "MODEL" + `"
)
var _ = modelfailure.Write
var _ = httperr.Write
type Service struct{ lane model.Client }
func (s *Service) Draft(r *http.Request) (string, error) {
	res, err := s.lane.Complete(r.Context(), model.Request{})
	return res.Text, err
}
type Handlers struct{ svc *Service }
func (h Handlers) DraftEmail(w http.ResponseWriter, r *http.Request) {
	draft, err := h.svc.Draft(r)
	if err != nil { WRITER(w, r, err); return }
	httperr.WriteJSON(w, http.StatusOK, draft)
}`
	for writer, want := range map[string]int{"httperr.Write": 1, "modelfailure.Write": 0} {
		t.Run(writer, func(t *testing.T) {
			src := strings.NewReplacer("MODEL", modelPortPath, "WRITER", writer).Replace(source)
			got := modelFailureCensus(t, []*typeCheckedSources{typeCheckPlantedAs(t, "compose/planted", src)})
			if got.reaching != 1 || len(got.findings) != want {
				t.Fatalf("reaching = %d, findings = %q; want 1 handler reaching a model and %d finding",
					got.reaching, got.findings, want)
			}
			if want == 1 && !strings.HasPrefix(got.findings[0], "(compose/planted.Handlers).DraftEmail ") {
				t.Errorf("the finding names %q, not the planted handler", got.findings[0])
			}
		})
	}
}

// The census only means something if it can say no: to a direct model call,
// to one made through a value handed to another package, and to one reached
// through an interface this package implements.
func TestTheModelFailureCensusNamesAHandlerAnsweringA500(t *testing.T) {
	prelude := "package planted\nimport (\n\t\"net/http\"\n\t\"github.com/margince/margince/backend/internal/modules/ai\"\n" +
		"\t\"github.com/margince/margince/backend/internal/compose/modelfailure\"\n" +
		"\t\"github.com/margince/margince/backend/internal/platform/httperr\"\n" +
		"\t\"github.com/margince/margince/backend/internal/platform/vectorkit\"\n" +
		"\t\"github.com/margince/margince/backend/internal/modules/knowledge\"\n" +
		"\tmodel \"" + modelPortPath + "\"\n)\n" +
		"var _ = modelfailure.Write\nvar _ = httperr.Write\nvar _ ai.Validator\nvar _ vectorkit.Embedder\nvar _ *knowledge.Store\nvar _ model.Request\n"
	for name, tc := range map[string]struct {
		body              string
		reaches, answered bool
	}{
		"a direct call answered as a 500": {`func h(w http.ResponseWriter, r *http.Request, c model.Client) {
	if _, err := ai.Ask(r.Context(), c, model.Request{}, nil); err != nil { httperr.Write(w, r, err) }
}`, true, false},
		"a value handed to another package": {`func h(w http.ResponseWriter, r *http.Request, s *knowledge.Store, e vectorkit.Embedder) {
	if _, _, err := s.Retrieve(r.Context(), [16]byte{}, "q", e); err != nil { httperr.Write(w, r, err) }
}`, true, false},
		"an interface this package implements": {`type asker interface{ ask(r *http.Request) error }
type engine struct{ c model.Client }
func (e engine) ask(r *http.Request) error { _, err := e.c.Complete(r.Context(), model.Request{}); return err }
func h(w http.ResponseWriter, r *http.Request, a asker) { if err := a.ask(r); err != nil { httperr.Write(w, r, err) } }`, true, false},
		"answered through modelfailure": {`func h(w http.ResponseWriter, r *http.Request, c model.Client) {
	if _, err := ai.Ask(r.Context(), c, model.Request{}, nil); err != nil { modelfailure.Write(w, r, err) }
}`, true, true},
		"no model at all": {`func h(w http.ResponseWriter, r *http.Request) { httperr.Write(w, r, nil) }`, false, false},
	} {
		t.Run(name, func(t *testing.T) {
			var found *handlerUnit
			for _, unit := range modelHandlerUnits(typeCheckPlanted(t, prelude+tc.body)) {
				if unit.name == "planted.h" {
					found = &unit
				}
			}
			if found == nil {
				t.Fatal("the census did not see the planted handler")
			}
			if found.reachesModel != tc.reaches || found.answered() != tc.answered {
				t.Errorf("reaches a model = %v, answers through modelfailure = %v; want %v, %v",
					found.reachesModel, found.answered(), tc.reaches, tc.answered)
			}
		})
	}
}

// handlerUnit is one HTTP handler — a declared function or a function literal
// taking a ResponseWriter and a Request — and what its calls can reach.
type handlerUnit struct {
	name         string
	reachesModel bool
	// modelCalls counts the calls in the handler that can reach a model, and
	// unanswered names each model call, here or in a helper beneath, whose
	// error misses modelfailure.Write.
	modelCalls int
	unanswered []modelDropSite
}

// modelDropSite is where a model call's error is dropped, and the function —
// the handler, or a helper beneath it — that drops it.
type modelDropSite struct{ at, owner string }

// answered reports whether the handler reaches a model and answers every
// model-reaching call's error through modelfailure.Write. A handler that
// reaches a model through no call the census can follow is not: its error
// goes somewhere the census cannot see.
func (u handlerUnit) answered() bool {
	return u.modelCalls > 0 && len(u.unanswered) == 0
}

func (u handlerUnit) finding() string {
	if u.modelCalls == 0 {
		return u.name + " reaches a model through no call whose error the census can follow — " +
			"call the model-reaching function directly and answer its error through modelfailure.Write"
	}
	sites := make([]string, 0, len(u.unanswered))
	for _, drop := range u.unanswered {
		sites = append(sites, drop.at+" (in "+drop.owner+")")
	}
	return u.name + " reaches a model, and the error of the call at " + strings.Join(sites, ", ") +
		" never reaches modelfailure.Write, so a provider that is down reaches its client as an " +
		"opaque 500 — write that error through modelfailure.Write, or name the function dropping it in " +
		"modelHandlersThatDegrade with why its answer without the model is honest"
}

// modelHandlerUnits finds every handler in the checked package and follows its
// calls.
func modelHandlerUnits(checked *typeCheckedSources) []handlerUnit {
	graph := newModelCallGraph(checked)
	var units []handlerUnit
	for _, file := range checked.files {
		ast.Inspect(file, func(n ast.Node) bool {
			sig, body, name := handlerOf(checked, n)
			if sig == nil || !isHandlerSignature(sig) {
				return true
			}
			unit := handlerUnit{name: name}
			for _, callee := range graph.refs(body) {
				unit.reachesModel = unit.reachesModel || graph.visit(callee).model
			}
			var unanswered []modelDrop
			unit.modelCalls, unanswered = graph.unansweredModelCalls(body, sig)
			for _, drop := range unanswered {
				at := checked.fset.Position(drop.at)
				owner := name
				if drop.owner != nil {
					owner = drop.owner.FullName()
				}
				unit.unanswered = append(unit.unanswered, modelDropSite{
					at: fmt.Sprintf("%s:%d", filepath.Base(at.Filename), at.Line), owner: owner,
				})
			}
			units = append(units, unit)
			return true
		})
	}
	return units
}

// handlerOf is a function node's signature, body and name, or a nil signature.
func handlerOf(checked *typeCheckedSources, n ast.Node) (*types.Signature, *ast.BlockStmt, string) {
	switch n := n.(type) {
	case *ast.FuncDecl:
		if obj, ok := checked.info.Defs[n.Name].(*types.Func); ok && n.Body != nil {
			return obj.Signature(), n.Body, obj.FullName()
		}
	case *ast.FuncLit:
		if sig, ok := checked.info.TypeOf(n).(*types.Signature); ok {
			return sig, n.Body, checked.fset.Position(n.Pos()).String()
		}
	}
	return nil, nil, ""
}

// reach is what one function's calls can get to.
type reach struct{ model bool }

// modelCallGraph resolves the package's functions to their bodies and memoises
// what each can reach, which of their parameters reach modelfailure.Write, and
// how each body's values flow.
type modelCallGraph struct {
	checked *typeCheckedSources
	bodies  map[*types.Func]*ast.BlockStmt
	memo    map[*types.Func]*reach
	sinks   map[sinkKey]bool
	flows   map[*ast.BlockStmt]*modelErrorFlow
	// declared holds the bodies of declared functions, the only ones whose
	// return has a caller the census can follow; swallowed memoises, per
	// function, the model calls in it whose error goes nowhere.
	declared  map[*ast.BlockStmt]bool
	swallowed map[*types.Func][]modelDrop
}

func newModelCallGraph(checked *typeCheckedSources) *modelCallGraph {
	g := &modelCallGraph{
		checked: checked, bodies: map[*types.Func]*ast.BlockStmt{}, memo: map[*types.Func]*reach{},
		sinks: map[sinkKey]bool{}, flows: map[*ast.BlockStmt]*modelErrorFlow{},
		declared: map[*ast.BlockStmt]bool{}, swallowed: map[*types.Func][]modelDrop{},
	}
	for _, file := range checked.files {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
				if obj, ok := checked.info.Defs[fn.Name].(*types.Func); ok {
					g.bodies[obj] = fn.Body
					g.declared[fn.Body] = true
				}
			}
		}
	}
	return g
}

// refs lists the functions a body names, called or passed as a value.
func (g *modelCallGraph) refs(body ast.Node) []*types.Func {
	var out []*types.Func
	ast.Inspect(body, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			if f, ok := g.checked.info.Uses[id].(*types.Func); ok {
				out = append(out, f.Origin())
			}
		}
		return true
	})
	return out
}

// visit reports what f can reach. The memo is entered before the walk, so a
// recursive call reads the partial answer rather than looping.
func (g *modelCallGraph) visit(f *types.Func) reach {
	if r, ok := g.memo[f]; ok {
		return *r
	}
	r := &reach{}
	g.memo[f] = r
	if isModelSeed(f, g.checked.pkg) {
		r.model = true
		return *r
	}
	callees := implementationsOf(f, g.checked.pkg)
	if body, ok := g.bodies[f]; ok {
		callees = append(callees, g.refs(body)...)
	}
	for _, callee := range callees {
		r.model = r.model || g.visit(callee).model
	}
	return *r
}

func isHandlerSignature(sig *types.Signature) bool {
	var w, r bool
	for p := range sig.Params().Variables() {
		switch types.TypeString(p.Type(), nil) {
		case "net/http.ResponseWriter":
			w = true
		case "*net/http.Request":
			r = true
		}
	}
	return w && r
}

// modelPortPath and modelFailurePath are derived from the types they name, so a
// move of either package moves the census with it.
var (
	modelPortPath    = reflect.TypeFor[model.Request]().PkgPath()
	modelFailurePath = modelPortPath[:len(modelPortPath)-len("shared/ports/model")] + "compose/modelfailure"
)

func isModelFailureWrite(f *types.Func) bool {
	return f.Pkg() != nil && f.Pkg().Path() == modelFailurePath && f.Name() == "Write"
}

// isModelSeed reports whether a call to f leaves the package into a model call:
// any interface method, or any function elsewhere, that takes a model request;
// a function elsewhere taking a value able to make one; or a method elsewhere
// on a type holding one.
func isModelSeed(f *types.Func, pkg *types.Package) bool {
	sig := f.Signature()
	inside := f.Pkg() == pkg
	if inside && (sig.Recv() == nil || !types.IsInterface(sig.Recv().Type())) {
		return false
	}
	for p := range sig.Params().Variables() {
		if isModelRequest(p.Type()) || (!inside && carriesModelCall(p.Type())) {
			return true
		}
	}
	return !inside && holdsModelCall(f)
}

func isModelRequest(t types.Type) bool {
	if ptr, ok := types.Unalias(t).(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := types.Unalias(t).(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == modelPortPath &&
		(named.Obj().Name() == "Request" || named.Obj().Name() == "EmbedRequest")
}

// carriesModelCall reports whether t is an interface with a method taking a
// model request — a model client, an embedder, a completer.
func carriesModelCall(t types.Type) bool {
	iface, ok := types.Unalias(t).Underlying().(*types.Interface)
	if !ok {
		return false
	}
	for m := range iface.Methods() {
		for p := range m.Signature().Params().Variables() {
			if isModelRequest(p.Type()) {
				return true
			}
		}
	}
	return false
}

// holdsModelCall reports whether a method's receiver is a struct with a field
// able to make a model call: another package's engine, wired with the router
// inside it.
func holdsModelCall(f *types.Func) bool {
	recv := f.Signature().Recv()
	if recv == nil {
		return false
	}
	t := recv.Type()
	if ptr, ok := types.Unalias(t).(*types.Pointer); ok {
		t = ptr.Elem()
	}
	st, ok := types.Unalias(t).Underlying().(*types.Struct)
	if !ok {
		return false
	}
	for field := range st.Fields() {
		if carriesModelCall(field.Type()) {
			return true
		}
	}
	return false
}

// implementationsOf lists the methods of this package's own types that a call
// through f, an interface method, may dispatch to.
func implementationsOf(f *types.Func, pkg *types.Package) []*types.Func {
	recv := f.Signature().Recv()
	if recv == nil {
		return nil
	}
	iface, ok := recv.Type().Underlying().(*types.Interface)
	if !ok {
		return nil
	}
	var out []*types.Func
	for _, name := range pkg.Scope().Names() {
		tn, ok := pkg.Scope().Lookup(name).(*types.TypeName)
		if !ok || types.IsInterface(tn.Type()) {
			continue
		}
		for _, t := range []types.Type{tn.Type(), types.NewPointer(tn.Type())} {
			if !types.Implements(t, iface) {
				continue
			}
			if m, _, _ := types.LookupFieldOrMethod(t, true, pkg, f.Name()); m != nil {
				if mf, ok := m.(*types.Func); ok {
					out = append(out, mf.Origin())
				}
			}
			break
		}
	}
	return out
}
