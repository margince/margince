// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

//go:build !integration

package gates

// An operation that declares the shared `Sort` parameter has a handler that
// reads it.
//
// `GET /partners` declared `Sort` and nothing behind it read one: the handler
// mapped `Limit`, `PartnerRole`, `CertStatus` and `Cursor` and dropped
// `params.Sort` on the floor, while the store paged on a bare `company_id`
// keyset. So `?sort=name` answered 200 with rows in uuid order — nothing
// failed, and a client that trusted the operation was told an ordering the
// list does not have. That one is fixed; this is what stops the next.
//
// THE CORPUS IS THE CONTRACT, which is the whole point. The integration suite
// that exists for this class could not see the defect because its cases are a
// hand-written list per resource and partners was not in it — a gate that
// hard-codes part of its subject has become a second copy of it. Here an
// operation is enrolled the moment it declares the parameter, so the next
// resource to offer a sort is asked about it without anybody remembering.
//
// WHAT IT CANNOT SEE. That the sort is CORRECT — that `?sort=name` really
// orders by name rather than reading the field and ignoring it. That is a
// behavioural claim and needs seeded rows; `listsortcoverage_integration_test.go`
// makes it per resource. This gate holds the floor beneath it: a handler that
// never names the parameter cannot be honouring it, and that is the failure
// that shipped.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// sortParameterRef is how an operation declares the shared parameter. Matched
// as the $ref it is, because a locally-spelled `name: sort` would be a
// different declaration with its own vocabulary and is not what this holds.
const sortParameterRef = "#/components/parameters/Sort"

func TestEveryOperationDeclaringSortHasAHandlerThatReadsIt(t *testing.T) {
	t.Parallel()
	declared := operationsDeclaringSort(t)
	if len(declared) == 0 {
		t.Fatal("no operation declares the shared Sort parameter — this census read a contract it does not recognise, and an empty corpus reports PASS")
	}

	readers := handlersNamingSort(t)
	for _, op := range declared {
		method := strings.ToUpper(op[:1]) + op[1:]
		handler, found := readers[method]
		if !found {
			t.Errorf("%s declares the shared Sort parameter and this census found no handler %s to read it — "+
				"either the handler is named something else, in which case this gate is looking in the wrong "+
				"place, or the operation is served by nothing", op, method)
			continue
		}
		if !handler {
			t.Errorf("%s declares the shared Sort parameter and its handler never names it, so `?sort=…` answers "+
				"200 in whatever order the store already used. Read it into the list input, or take the "+
				"parameter off the operation — a sort nobody honours is a promise to a client that the list "+
				"does not keep", op)
		}
	}
}

// operationsDeclaringSort reads the contract for the operations that offer the
// shared parameter, in a stable order so a failing run names them the same way
// twice.
func operationsDeclaringSort(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	// The method map is decoded as NODES: a path item carries a `parameters`
	// sequence of its own beside its methods, and a value type that expected
	// an operation there would fail to parse the contract rather than skip it.
	var doc struct {
		Paths map[string]map[string]yaml.Node `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	shared := sharedSortIdentity(t, raw)
	var out []string
	for path, methods := range doc.Paths {
		// INHERITED parameters count. OpenAPI lets a path item declare a
		// parameter list shared by all its methods, so an operation can offer
		// the sort without naming it — and a census that read only the
		// operation level would leave that one enrolled nowhere while the
		// others kept it reporting a healthy total. Nothing in this contract
		// inherits Sort today, which is exactly why it is worth reading: the
		// first one to do so must not be the one that discovers the gap.
		inherited := declaresSharedSort(t, methods["parameters"], shared)
		for method, node := range methods {
			if method == "parameters" {
				continue
			}
			var op struct {
				OperationID string      `yaml:"operationId"` //nolint:tagliatelle // OpenAPI's key, not ours to rename
				Parameters  []yaml.Node `yaml:"parameters"`
			}
			if err := node.Decode(&op); err != nil {
				t.Fatalf("reading %s %s out of the contract: %v", strings.ToUpper(method), path, err)
			}
			if op.OperationID == "" {
				continue
			}
			if declaresSharedSort(t, node, shared) || (inherited && !overridesSharedSort(t, node, shared)) {
				out = append(out, op.OperationID)
			}
		}
	}
	sort.Strings(out)
	return out
}

// parameterIdentity is what OpenAPI keys a parameter by, and therefore what an
// operation has to repeat to override one it would otherwise inherit.
type parameterIdentity struct {
	Name string `yaml:"name"`
	In   string `yaml:"in"`
}

// sharedSortIdentity reads the shared component's own name and location rather
// than assuming them, so an override is recognised by what the contract says
// the parameter IS.
func sharedSortIdentity(t *testing.T, raw []byte) parameterIdentity {
	t.Helper()
	var doc struct {
		Components struct {
			Parameters map[string]parameterIdentity `yaml:"parameters"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing the contract's components: %v", err)
	}
	shared, declared := doc.Components.Parameters["Sort"]
	if !declared || shared.Name == "" || shared.In == "" {
		t.Fatalf("the contract declares no shared Sort parameter to census against (read %+v)", shared)
	}
	return shared
}

// declaresSharedSort reports whether a parameter list — a path item's or an
// operation's — carries the shared component.
func declaresSharedSort(t *testing.T, node yaml.Node, _ parameterIdentity) bool {
	t.Helper()
	for _, param := range parameterList(t, node) {
		var ref struct {
			Ref string `yaml:"$ref"`
		}
		// A parameter is either a $ref or an inline declaration; only the
		// first can be the shared one, and a decode that finds no $ref simply
		// leaves it empty.
		if err := param.Decode(&ref); err == nil && ref.Ref == sortParameterRef {
			return true
		}
	}
	return false
}

// overridesSharedSort reports whether an operation redeclares the inherited
// parameter as its own — which takes it out of this census, because what it
// then offers is a vocabulary of its own rather than the shared one.
func overridesSharedSort(t *testing.T, node yaml.Node, shared parameterIdentity) bool {
	t.Helper()
	for _, param := range parameterList(t, node) {
		var own parameterIdentity
		if err := param.Decode(&own); err == nil && own == shared {
			return true
		}
	}
	return false
}

// parameterList reads the `parameters` sequence off a path item or an
// operation, answering empty for a node that carries none.
func parameterList(t *testing.T, node yaml.Node) []yaml.Node {
	t.Helper()
	if node.Kind == 0 {
		return nil
	}
	if node.Kind == yaml.SequenceNode {
		var list []yaml.Node
		if err := node.Decode(&list); err != nil {
			t.Fatalf("reading a parameter list out of the contract: %v", err)
		}
		return list
	}
	var holder struct {
		Parameters []yaml.Node `yaml:"parameters"`
	}
	if err := node.Decode(&holder); err != nil {
		t.Fatalf("reading a parameter list out of the contract: %v", err)
	}
	return holder.Parameters
}

// handlersNamingSort walks the tree for HTTP handlers and reports, per handler
// name, whether the sort parameter reaches anything.
//
// Named rather than matched by receiver type: the generated ServerInterface
// method IS the operationId capitalised, which is what lets the corpus come
// from the contract and the answer from the code without a map between them.
//
// ONE HOP, because the honouring shape is not always in the handler. Deal
// rooms map their whole parameter set in a package-level `listInput(params)`
// and the handler's own body names nothing; reading the handler alone reports
// that operation broken while it works, and a census that fails a working
// surface is worse than no census — it teaches a reader to disbelieve it.
func handlersNamingSort(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, f := range listSortHandlerScope.Files(t) {
		byName := packageFunctions(t, f.Path)
		for _, decl := range f.File.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Recv == nil || fn.Body == nil || !servesHTTP(fn) {
				continue
			}
			out[fn.Name.Name] = readsTheSortParameter(fn, byName)
		}
	}
	return out
}

// packageFunctions indexes the plain functions declared beside a handler, so
// the hop below can be resolved without a type checker. Methods are left out:
// the shape this follows is a package-level mapper, and a method call would
// need a receiver this cannot resolve from syntax alone.
func packageFunctions(t *testing.T, path string) map[string]*ast.FuncDecl {
	t.Helper()
	out := map[string]*ast.FuncDecl{}
	dir := filepath.Dir(path)
	sources, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("listing %s: %v", dir, err)
	}
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), source, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", source, err)
		}
		for _, decl := range file.Decls {
			if fn, isFunc := decl.(*ast.FuncDecl); isFunc && fn.Recv == nil && fn.Body != nil {
				out[fn.Name.Name] = fn
			}
		}
	}
	return out
}

// readsTheSortParameter reports whether the handler reads its sort parameter,
// itself or through one package-level function it hands the whole parameter
// set to.
func readsTheSortParameter(handler *ast.FuncDecl, byName map[string]*ast.FuncDecl) bool {
	params := handlerParamsName(handler)
	if params == "" {
		// A handler that does not name its parameter set cannot read a field
		// off it, and cannot pass it on either.
		return false
	}
	if namesSortOn(handler.Body, params) {
		return true
	}
	hopped := false
	ast.Inspect(handler.Body, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall || hopped {
			return !hopped
		}
		callee, named := call.Fun.(*ast.Ident)
		if !named {
			return true
		}
		target, known := byName[callee.Name]
		if !known || target.Body == nil {
			return true
		}
		for at, arg := range call.Args {
			ident, isIdent := arg.(*ast.Ident)
			if !isIdent || ident.Name != params {
				continue
			}
			if inner := nthParamName(target, at); inner != "" && namesSortOn(target.Body, inner) {
				hopped = true
			}
		}
		return !hopped
	})
	return hopped
}

// handlerParamsName is the name the handler gave its parameter set — the third
// argument of the generated signature, which a handler that takes none writes
// as `_`.
func handlerParamsName(fn *ast.FuncDecl) string {
	return nthParamName(fn, 2)
}

// nthParamName answers the identifier bound to one parameter position, or ""
// for a position that is absent or discarded.
func nthParamName(fn *ast.FuncDecl, at int) string {
	if fn.Type.Params == nil {
		return ""
	}
	position := 0
	for _, field := range fn.Type.Params.List {
		names := field.Names
		if len(names) == 0 {
			names = []*ast.Ident{{Name: "_"}}
		}
		for _, name := range names {
			if position == at {
				if name.Name == "_" {
					return ""
				}
				return name.Name
			}
			position++
		}
	}
	return ""
}

// namesSortOn reports whether a body reads the `Sort` field off the named
// parameter set.
//
// Read off the syntax rather than off the text: a handler that mentions the
// word in a comment explaining why it does NOT honour a sort would satisfy a
// text match, and that comment is exactly what a handler dropping the
// parameter tends to grow.
func namesSortOn(body *ast.BlockStmt, params string) bool {
	written := assignedSortTargets(body, params)
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		selector, ok := n.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Sort" || written[selector] {
			return !found
		}
		if ident, isIdent := selector.X.(*ast.Ident); isIdent && ident.Name == params {
			found = true
		}
		return !found
	})
	return found
}

// assignedSortTargets collects the `params.Sort` selectors that are WRITTEN
// rather than read.
//
// A handler that clears the caller's sort — `params.Sort = nil` — names the
// field and honours nothing, which is the one shape that would satisfy this
// census while doing the exact thing it exists to catch. Only the left side of
// a plain assignment is excluded: `in.Sort = params.Sort` reads on the right,
// and `params.Sort = something(params.Sort)` reads there too.
func assignedSortTargets(body *ast.BlockStmt, params string) map[*ast.SelectorExpr]bool {
	written := map[*ast.SelectorExpr]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		assign, isAssign := n.(*ast.AssignStmt)
		if !isAssign || assign.Tok != token.ASSIGN {
			return true
		}
		for _, target := range assign.Lhs {
			selector, isSelector := target.(*ast.SelectorExpr)
			if !isSelector || selector.Sel.Name != "Sort" {
				continue
			}
			if ident, isIdent := selector.X.(*ast.Ident); isIdent && ident.Name == params {
				written[selector] = true
			}
		}
		return true
	})
	return written
}

// servesHTTP reports whether this method is an HTTP handler, by the signature
// every generated route takes: a ResponseWriter and a Request.
func servesHTTP(fn *ast.FuncDecl) bool {
	if fn.Type.Params == nil || len(fn.Type.Params.List) < 2 {
		return false
	}
	return strings.Contains(typeString(fn.Type.Params.List[0].Type), "ResponseWriter") &&
		strings.Contains(typeString(fn.Type.Params.List[1].Type), "Request")
}

func typeString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.SelectorExpr:
		return t.Sel.Name
	case *ast.StarExpr:
		return typeString(t.X)
	case *ast.Ident:
		return t.Name
	}
	return ""
}

// listSortHandlerScope claims every HTTP handler lives under internal/. A
// handler outside it would be one this census cannot see, which is the
// direction that reads green over the defect.
var listSortHandlerScope = gatekit.Scope{
	Roots:   []string{"internal"},
	Subject: func(_ string, file *ast.File) bool { return declaresAnHTTPHandler(file) },
	Exempt:  gatekit.Waive(map[string]string{}),
}

func declaresAnHTTPHandler(file *ast.File) bool {
	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if isFunc && fn.Recv != nil && fn.Body != nil && servesHTTP(fn) {
			return true
		}
	}
	return false
}
