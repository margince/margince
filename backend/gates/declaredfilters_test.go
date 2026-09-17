// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A declared narrowing parameter is read by the handler it is declared on, or
// it is not declared.
//
// The defect this closes is the widest wrong answer there is. `GET
// /v1/contacts?tag=vip` whose handler never reads `tag` returns EVERY contact the
// caller may see, with 200 OK and a well-formed page: the client cannot tell
// that from a workspace where everyone carries the tag. Three of these were
// live when the gate was written — `tag`, `company.domain`,
// `activity.assignee_id` — plus one shadow, `deal.project_id`, that answered
// every deal in the workspace to a caller who named one project.
//
// It is derived rather than listed. The generated contract carries every list
// operation's query parameters as a `*Params` struct, so the census IS the
// contract: a parameter added upstream lands here as a failure until a handler
// reads it. That is the half a hand-maintained list cannot do, and the half
// that matters — every one of these defects was a parameter someone declared
// and nobody came back for.
//
// The obligation: a handler READS the dial — or hands the whole params value on
// to something that does, or owns the raw query string itself (the report
// handlers parse it wholesale).
//
// READ, precisely — not "correctly bound". This walk proves a handler NAMES
// the parameter; it cannot prove the value then narrows the query, and a
// handler that mapped `tag` onto the wrong field would satisfy it. That is
// the honest limit of a static census and it is stated rather than papered
// over, because the pair is what covers the class: this gate is the half no
// test can be written for in advance — the parameter nobody came back for —
// and declaredfilters_http_integration_test.go is the half that proves the
// named ones actually narrow, over HTTP, against records created over HTTP.
// A new parameter therefore arrives with the census already failing, and its
// wire arm is what closes the finding.
//
// Paging and ordering are deliberately OUT of scope: `cursor`, `limit` and
// `sort` are the shared components a page's SHAPE is spelled with, not its
// membership, and a handler that ignores one answers a differently-shaped page
// rather than a different question. Six operations ignore one of the three
// today; that is its own obligation with its own fix, filed separately.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	// contractParamsSource holds the generated `*Params` structs — the census
	// of what the contract declares a read may be narrowed by.
	contractParamsSource = "internal/contracts/api_gen.go"

	// contractsPackageAlias is how every handler spells the generated contract
	// package, which is what makes a params type recognisable in a signature.
	contractsPackageAlias = "crmcontracts"

	// wantMinimumNarrowedOperations guards the way this gate fails silently: a
	// census that stops recognising the generated shape finds no parameters,
	// judges nothing, and reads exactly like a clean tree. Fifty operations
	// declare a narrowing parameter today; the floor sits well below that, so
	// retiring one stays an ordinary change and only a collapse is a finding.
	wantMinimumNarrowedOperations = 30
)

// unjudgedParameterTypes are the generated types this gate does not judge.
//
// Recognising them by TYPE rather than by name is what keeps the exclusion
// honest: it is the contract's own component reuse that says a parameter is a
// page's shape, and a filter that happened to be called `sort` on some
// operation would still be judged.
//
// `Cursor` and `Limit` are NOT here any more. They were scoped out with `Sort`
// as "a page's shape rather than its membership", and the distinction is real —
// a dropped filter answers a wider question than the one asked, where a dropped
// page dial answers the right question in the wrong shape. But the second is
// still a claim the surface does not keep: `?limit=5` returning the whole set
// means the caller sized a page and got a catalog, and `?cursor=` accepted and
// inert is the shape that walks page one forever.
//
// There is deliberately NO waiver list beside this. An operation that will not
// honour a dial removes the declaration from the contract instead — the honest
// state after a retirement is that the parameter is gone, not that it is
// declared and excused.
//
// `Sort` is not here either, and the empty map is the state that says so: every
// operation declaring a sort now honours it. It was the last of the three page
// dials to be scoped out, because honouring one is a per-store change rather
// than a contract edit — a sort vocabulary the store publishes, and a keyset
// cursor carrying the field it was minted for.
//
// The map stays rather than the check losing its type dimension: it is where a
// future component-typed parameter is scoped out, with the reason, and an empty
// one is the honest reading of the tree today.
var unjudgedParameterTypes = map[string]bool{}

// declaredFilter is one narrowing query parameter of one operation: the Go
// field a handler reads, and the name a caller types.
type declaredFilter struct {
	field string
	wire  string
}

// TestEveryDeclaredNarrowingParameterIsReadByItsHandler is the native obligation.
func TestEveryDeclaredNarrowingParameterIsReadByItsHandler(t *testing.T) {
	t.Parallel()
	declared := narrowingParametersByType(t)
	if len(declared) < wantMinimumNarrowedOperations {
		t.Fatalf("only %d operations declare a narrowing query parameter in %s, want at least %d — "+
			"the census is reading the wrong shape and would judge nothing",
			len(declared), contractParamsSource, wantMinimumNarrowedOperations)
	}

	handlers := 0
	for _, file := range declaredFilterScope(declared).Files(t) {
		for _, h := range paramsHandlersIn(file, declared) {
			handlers++
			for _, missed := range h.unread {
				t.Errorf("%s: %s declares `%s` and %s never reads it, so the page it answers is wider "+
					"than the one that was asked for — bind the parameter to the read, or retire it "+
					"from the contract",
					file.Path, h.paramsType, missed, h.name)
			}
		}
	}
	if handlers == 0 {
		t.Fatal("no handler taking a narrowing params value was found: the walk is judging nothing")
	}
}

// TestTheDeclaredFilterWalkReportsADroppedParameterAndNothingElse is the gate's
// own defect test. Both obligations above pass today, and a walk that reported
// nothing because it recognises nothing would pass identically — so each shape
// the walk must tell apart is put to it here, against source written for the
// purpose rather than against the tree it judges.
func TestTheDeclaredFilterWalkReportsADroppedParameterAndNothingElse(t *testing.T) {
	t.Parallel()
	declared := map[string][]declaredFilter{"ListThingsParams": {{field: "Tag", wire: "tag"}}}
	for _, probe := range []struct {
		name   string
		body   string
		unread []string
	}{
		{
			"a dropped parameter is reported",
			`in := Input{Limit: params.Limit}; use(in)`,
			[]string{"tag"},
		},
		{
			"a parameter the handler reads is not",
			`use(Input{Tag: params.Tag})`, nil,
		},
		{
			"a params value handed on whole is not",
			`h.other(w, r, params)`, nil,
		},
		{
			"a handler owning the raw query string is not",
			`use(parse(r.URL.Query()))`, nil,
		},
	} {
		t.Run(probe.name, func(t *testing.T) {
			file := parseGoSource(t, "package p\nfunc (h H) ListThings(w W, r R, params crmcontracts.ListThingsParams) {\n"+
				probe.body+"\n}\n")
			handlers := paramsHandlersIn(file, declared)
			if len(handlers) != 1 {
				t.Fatalf("the walk found %d handlers, want 1 — it is not recognising the signature at all", len(handlers))
			}
			if got := strings.Join(handlers[0].unread, ","); got != strings.Join(probe.unread, ",") {
				t.Errorf("unread = %q, want %q", got, strings.Join(probe.unread, ","))
			}
		})
	}
}

// TestTheDeclaredFilterCensusReadsTheGeneratedShape pins the other half of the
// silent failure: a census that mistook the paging components for filters, or
// missed a form tag, would judge the wrong set without ever reporting it.
func TestTheDeclaredFilterCensusReadsTheGeneratedShape(t *testing.T) {
	t.Parallel()
	declared := narrowingParametersByType(t)
	contacts, listed := declared["ListContactsParams"]
	if !listed {
		t.Fatalf("the census carries no ListContactsParams — it is reading the wrong shape")
	}
	var wire []string
	for _, filter := range contacts {
		wire = append(wire, filter.wire)
	}
	// The contact list declares exactly these. All three page dials are among
	// them — the gate judges a dial the same way it judges a filter, and none
	// of the three is scoped out any more.
	want := "ai_written,captured_by_kind,company_id,cursor,include_archived,limit,owner_id," +
		"owner_team_id,q,sort,tag_id,tag_mode,unassigned"
	if got := strings.Join(wire, ","); got != want {
		t.Errorf("listContacts's narrowing parameters = %q, want %q", got, want)
	}
}

// declaredFilterScope proves the roots hold every handler this gate judges: a
// params value taken anywhere else would be a read the gate never sees.
func declaredFilterScope(declared map[string][]declaredFilter) gatekit.Scope {
	return gatekit.Scope{
		Roots: []string{modulesDir, composeTier},
		Subject: func(_ string, file *ast.File) bool {
			return len(paramsHandlersIn(gatekit.ParsedFile{File: file}, declared)) > 0
		},
	}
}

// paramsHandler is one handler judged: which operation's params it takes, and
// which of that operation's narrowing parameters it never reads.
type paramsHandler struct {
	name       string
	paramsType string
	unread     []string
}

// paramsHandlersIn walks one file for handlers taking a declared params value
// and reports what each leaves unread, by wire name and in declaration order.
func paramsHandlersIn(file gatekit.ParsedFile, declared map[string][]declaredFilter) []paramsHandler {
	var out []paramsHandler
	for _, decl := range file.File.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		ident, paramsType := paramsArgument(fn, declared)
		if paramsType == "" {
			continue
		}
		read, wholesale := parameterReferences(fn.Body, ident)
		var unread []string
		if !wholesale {
			for _, filter := range declared[paramsType] {
				if !read[filter.field] {
					unread = append(unread, filter.wire)
				}
			}
		}
		out = append(out, paramsHandler{name: fn.Name.Name, paramsType: paramsType, unread: unread})
	}
	return out
}

// paramsArgument finds the handler's params argument: the identifier it is
// bound to (empty for `_`, which reads nothing at all) and the generated type
// it carries. A signature naming no declared params type answers "".
func paramsArgument(fn *ast.FuncDecl, declared map[string][]declaredFilter) (ident, paramsType string) {
	for _, field := range fn.Type.Params.List {
		// Through the pointer a handler MIGHT take it by, and past a group
		// binding two names at once: a signature this walk does not recognise
		// is a handler it silently never judges, and the floors below count
		// handlers rather than missing ones.
		expr := field.Type
		if pointer, isPointer := expr.(*ast.StarExpr); isPointer {
			expr = pointer.X
		}
		selector, ok := expr.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		pkg, ok := selector.X.(*ast.Ident)
		if !ok || pkg.Name != contractsPackageAlias || len(declared[selector.Sel.Name]) == 0 {
			continue
		}
		for _, name := range field.Names {
			if name.Name != "_" {
				return name.Name, selector.Sel.Name
			}
		}
		return "", selector.Sel.Name
	}
	return "", ""
}

// parameterReferences reports which fields of the params value the body reads
// by name, and whether it binds the value WHOLESALE — in which case the field
// set says nothing and the handler is accounted for either way.
//
// The two wholesale shapes exist because binding a parameter is not always a
// selector at the handler: a body that hands the whole params value on has
// passed every field to whatever maps them, and a body that parses the raw
// query string owns every parameter in it — which is how the report handlers
// read the reserved `by`/`agg` keys alongside a free-form vocabulary the
// generated struct cannot spell. Both are broader than their justification:
// ANY call taking the value counts, so a debug log naming `params` would
// exempt a handler. They are the native obligation's allowances and they are
// stated here rather than narrowed, because a walk that guessed which calls
// "really" bind would be the unreliable half of this gate.
func parameterReferences(body *ast.BlockStmt, ident string) (read map[string]bool, wholesale bool) {
	// A handler that discards the generated struct (`_ crmcontracts.XParams`)
	// reads no field by name, but it may still own the raw query string — so
	// the walk runs either way, and only the by-name half depends on there
	// being an identifier to match.
	read = map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.SelectorExpr:
			if x, ok := node.X.(*ast.Ident); ok && x.Name == ident {
				read[node.Sel.Name] = true
			}
		case *ast.CallExpr:
			wholesale = wholesale || readsRawQuery(node) || passesWhole(node, ident)
		}
		return true
	})
	return read, wholesale
}

// readsRawQuery reports whether the call is r.URL.Query(), the handler taking
// the whole query string rather than the generated struct.
func readsRawQuery(call *ast.CallExpr) bool {
	method, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || method.Sel.Name != "Query" {
		return false
	}
	url, ok := method.X.(*ast.SelectorExpr)
	return ok && url.Sel.Name == "URL"
}

// passesWhole reports whether the call hands the params value on unopened.
func passesWhole(call *ast.CallExpr, ident string) bool {
	for _, arg := range call.Args {
		if named, ok := arg.(*ast.Ident); ok && named.Name == ident {
			return true
		}
	}
	return false
}

// narrowingParametersByType is the census: every generated `*Params` struct,
// keyed by type name, carrying the query parameters that narrow what the
// operation answers. Paging and ordering are dropped by type, and a struct left
// with nothing is dropped with them.
func narrowingParametersByType(t *testing.T) map[string][]declaredFilter {
	t.Helper()
	out := map[string][]declaredFilter{}
	for _, decl := range parseGoFile(t, contractParamsSource).File.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || !strings.HasSuffix(typeSpec.Name.Name, "Params") {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}
			if filters := narrowingFields(structType); len(filters) > 0 {
				out[typeSpec.Name.Name] = filters
			}
		}
	}
	return out
}

// narrowingFields reads one params struct's query fields: the `form` tag names
// the wire parameter, and a field typed by one this gate does not judge
// (unjudgedParameterTypes) is not one of them.
func narrowingFields(structType *ast.StructType) []declaredFilter {
	var out []declaredFilter
	for _, field := range structType.Fields.List {
		if len(field.Names) != 1 || field.Tag == nil {
			continue
		}
		wire := formTagName(field.Tag.Value)
		if wire == "" || unjudgedParameterTypes[pointedToTypeName(field.Type)] {
			continue
		}
		out = append(out, declaredFilter{field: field.Names[0].Name, wire: wire})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].wire < out[j].wire })
	return out
}

// formTagName reads the query-parameter name out of a struct tag, or "" when
// the field carries none — a header or path parameter, which this gate is not
// about.
func formTagName(tag string) string {
	const marker = `form:"`
	start := strings.Index(tag, marker)
	if start < 0 {
		return ""
	}
	rest := tag[start+len(marker):]
	end := strings.IndexAny(rest, `",`)
	if end <= 0 {
		return ""
	}
	return rest[:end]
}

// pointedToTypeName names the type a field holds, through the pointer every
// optional parameter is generated as, and "" for anything qualified by a
// package — the shared paging components are all local to the generated file.
func pointedToTypeName(expr ast.Expr) string {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if named, ok := expr.(*ast.Ident); ok {
		return named.Name
	}
	return ""
}

// parseGoFile parses one source of the backend module, relative to its root.
func parseGoFile(t *testing.T, rel string) gatekit.ParsedFile {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), filepath.FromSlash(rel), nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", rel, err)
	}
	return gatekit.ParsedFile{Path: rel, File: file}
}

// parseGoSource parses source written for the walk's own defect test.
func parseGoSource(t *testing.T, src string) gatekit.ParsedFile {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "probe.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing the probe source: %v", err)
	}
	return gatekit.ParsedFile{Path: "probe.go", File: file}
}
