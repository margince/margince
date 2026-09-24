// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A handler waiting on a model answers a lane that did not answer as the lane
// being down (modelfailure.Write), never as an opaque 500. The set it binds is
// DERIVED: every handler in this package whose calls can reach a model call.
//
// "Can reach" is followed through this package's own functions and methods,
// through an interface method to every type here that implements it, and out
// of the package at the first call that takes a model request, takes a value
// able to make one, or is a method of a type holding one. The subpackages are
// not read; modelfailure's own doc says so.

import (
	"go/ast"
	"go/types"
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// modelHandlersThatDegrade are handlers that reach a model and deliberately
// answer without it, so a model failure never becomes their error.
var modelHandlersThatDegrade = gatekit.Waive(map[string]string{
	"(compose.Server).RegenerateOffer": "the mechanical revision is minted first and served; a drafting failure is logged and the caller keeps that revision",
})

func TestEveryHandlerReachingAModelAnswersThroughModelFailure(t *testing.T) {
	reaching := 0
	for _, unit := range modelHandlerUnits(typeCheckedComposeSources(t)) {
		if !unit.reachesModel {
			continue
		}
		reaching++
		if unit.writesModelFailure || modelHandlersThatDegrade.Waived(t, unit.name) {
			continue
		}
		t.Errorf("%s reaches a model but never answers through modelfailure.Write, so a provider that is down "+
			"reaches its client as an opaque 500 — write its error through modelfailure.Write", unit.name)
	}
	modelHandlersThatDegrade.AssertAllMatched(t)
	// The onboarding assistant, the cold start, the scrape, the site read and
	// the corpus ask all reach a model; fewer means the scan lost its way.
	if reaching < 5 {
		t.Fatalf("found %d handlers reaching a model — the census is reading the wrong files", reaching)
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
			if found.reachesModel != tc.reaches || found.writesModelFailure != tc.answered {
				t.Errorf("reaches a model = %v, answers through modelfailure = %v; want %v, %v",
					found.reachesModel, found.writesModelFailure, tc.reaches, tc.answered)
			}
		})
	}
}

// handlerUnit is one HTTP handler — a declared function or a function literal
// taking a ResponseWriter and a Request — and what its calls can reach.
type handlerUnit struct {
	name                             string
	reachesModel, writesModelFailure bool
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
				got := graph.visit(callee)
				unit.reachesModel = unit.reachesModel || got.model
				unit.writesModelFailure = unit.writesModelFailure || got.failure
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
type reach struct{ model, failure bool }

// modelCallGraph resolves the package's functions to their bodies and memoises
// what each can reach.
type modelCallGraph struct {
	checked *typeCheckedSources
	bodies  map[*types.Func]*ast.BlockStmt
	memo    map[*types.Func]*reach
}

func newModelCallGraph(checked *typeCheckedSources) *modelCallGraph {
	g := &modelCallGraph{checked: checked, bodies: map[*types.Func]*ast.BlockStmt{}, memo: map[*types.Func]*reach{}}
	for _, file := range checked.files {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
				if obj, ok := checked.info.Defs[fn.Name].(*types.Func); ok {
					g.bodies[obj] = fn.Body
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
	switch {
	case isModelFailureWrite(f):
		r.failure = true
		return *r
	case isModelSeed(f, g.checked.pkg):
		r.model = true
		return *r
	}
	callees := implementationsOf(f, g.checked.pkg)
	if body, ok := g.bodies[f]; ok {
		callees = append(callees, g.refs(body)...)
	}
	for _, callee := range callees {
		got := g.visit(callee)
		r.model = r.model || got.model
		r.failure = r.failure || got.failure
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
