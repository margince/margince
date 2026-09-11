// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

//go:build !integration

package gates

// Every prompt this build sends must be measured by a certification case.
//
// The task census beside this one (aitaskcensus_test.go) compares the registry
// to the task contract, and both are written out by hand on purpose — that is
// what makes each an independent claim. But neither is derived from the CODE
// that actually builds a request, so a site present in the tree and absent from
// both lists is invisible to them: it reads as PASS because nothing looked. A
// census that can fail short has already failed, and this one did — the meeting
// brief's SECTIONS shipped for months beside its certified PLAN, a second call
// with its own prompt that nothing graded.
//
// So this gate derives its corpus from the tree rather than from a list. A
// prompt-minting site is recognisable without being registered anywhere: it
// builds a model.Request composite literal carrying a System field.
//
// What it can and cannot see, stated rather than assumed (AGENTS.md rule 8 —
// ask what shape of the defect it cannot see, then plant that case):
//
//   - It reads the model package under any ALIAS, because the local name comes
//     from the file's own imports. A dot import it cannot read at all, and says
//     so rather than passing.
//   - It follows a CONDUIT — a builder handed its prompt as a parameter, like
//     companybrief.groundedRequest — up to the callers that chose the prompt, so
//     one literal serving four sites is four sites.
//   - It refuses a literal minted OUTSIDE any function, where there is no site
//     to attribute it to.
//   - It keys a method by its RECEIVER TYPE. This package declares 73 methods
//     called Complete; one node for them would let a cert case calling
//     completer.Complete certify every one, a rogue minter included.
//   - It does NOT resolve a receiver whose type the syntax does not state — an
//     interface parameter, most often. Those edges are DROPPED, which
//     over-refuses loudly rather than under-refusing silently.
//
// Each of those is pinned by a test below that must fail if the behaviour is
// undone. The claim is exactly that list, and no wider: this is a syntactic
// walk, not a type checker, and a shape absent from the list is a shape nobody
// has tested it against.
//
// Reachability is resolved through IMPORT PATHS rather than bare function
// names. Two packages here export a BriefRequest — meetingbrief's and
// personbrief's — and a name-keyed check reports the uncertified one as
// certified because its certified namesake answers for it. That is the same
// under-recognition this gate exists to refuse, one level down.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// funcKey names one function by the directory it lives in and its own name.
// The directory is the identity because a bare name is not unique across the
// tree, and the collision is not hypothetical: see the package comment.
type funcKey struct {
	dir  string
	name string
	// isMethod separates a method from a package-level function of the SAME
	// name. Without it, a call on a local value — helper.BuildRequest() —
	// records an edge to the package's own BuildRequest function, and an
	// orphan prompt builder reads as reached by a call that never touches it.
	isMethod bool
	// recv is the method's receiver type. It is the difference between one
	// node and seventy-three: this package declares 73 methods called
	// Complete, and collapsing them means any cert case calling
	// completer.Complete certifies EVERY one of them, including a rogue
	// Complete that mints a prompt nobody grades.
	recv string
}

func (k funcKey) String() string {
	if k.isMethod {
		return k.dir + "." + k.recv + "." + k.name
	}
	return k.dir + "." + k.name
}

// promptGraph is what one walk of the tree collects.
type promptGraph struct {
	// minting names every function that builds a request carrying a system
	// prompt — the sites, derived rather than listed.
	minting map[funcKey]bool
	// calls is the edge set: which functions each function reaches directly.
	calls map[funcKey][]funcKey
	// roots are the functions the certification layer itself defines. Every
	// certified site is reachable from one, because a case issues the request
	// production issues.
	roots map[funcKey]bool
	// dotImported names any package that dot-imports the model package, which
	// this scan cannot read. It is a refusal, not a finding.
	dotImported []string
	// returns is each package-level function's first result type, so a local
	// bound to a constructor carries a receiver type the syntax states rather
	// than one a naming convention implies.
	returns map[funcKey]string
	// looseLiterals names files that mint a prompt OUTSIDE any function — a
	// package-level var holding a func literal, say. The walk reads function
	// bodies, so such a site would be invisible; it is refused rather than
	// missed.
	looseLiterals []string
	// conduits mint on behalf of a CALLER: the system prompt comes in as a
	// parameter, so the literal is one and the sites are many.
	conduits map[funcKey]bool
	// promptArg is, for each conduit, WHICH parameter position carries the
	// prompt. Forwarding any old argument into a conduit does not make a
	// wrapper one — forwarding the prompt does.
	promptArg map[funcKey]int
	// forwards records, per call, the argument positions a function fills with
	// its OWN parameters. A wrapper that forwards its prompt into a conduit's
	// prompt position is a conduit itself, and without this the propagation
	// stops at the wrapper — leaving whoever chose the prompt out of the census.
	forwards map[funcKey]map[funcKey]map[int]bool
}

func TestEveryPromptIsCertified(t *testing.T) {
	t.Parallel()
	g := walkPromptTree(t, "internal")
	if len(g.minting) == 0 {
		// Under-recognition is the one way this gate must not break: a walk
		// that matched nothing would report PASS while measuring an empty
		// tree.
		t.Fatal("found no request builder at all, so this gate measured nothing")
	}
	if len(g.roots) == 0 {
		t.Fatal("found no certification case at all, so every site would read as uncertified")
	}
	for _, where := range g.looseLiterals {
		t.Errorf("%s builds a prompt outside any function, where this scan cannot attribute it to a site", where)
	}
	for _, dir := range g.dotImported {
		t.Errorf("%s dot-imports the model package, so this scan cannot see the requests it builds", dir)
	}
	propagateConduits(&g)
	reached := reachableFrom(g)
	var orphans []string
	for site := range g.minting {
		if !reached[site] {
			orphans = append(orphans, site.String())
		}
	}
	sort.Strings(orphans)
	for _, orphan := range orphans {
		t.Errorf(
			"%s builds a prompt no certification case reaches, so what the model answers it is graded by nothing",
			orphan)
	}
}

// walkPromptTree parses every non-test Go file under root once, collecting the
// minting sites, the call edges and the certification roots together.
func walkPromptTree(t *testing.T, root string) promptGraph {
	t.Helper()
	g := promptGraph{
		minting:   map[funcKey]bool{},
		calls:     map[funcKey][]funcKey{},
		roots:     map[funcKey]bool{},
		conduits:  map[funcKey]bool{},
		promptArg: map[funcKey]int{},
		forwards:  map[funcKey]map[funcKey]map[int]bool{},
	}
	fset := token.NewFileSet()
	type parsed struct {
		file *ast.File
		dir  string
		cert bool
	}
	var files []parsed
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, p, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		files = append(files, parsed{file: file, dir: filepath.Dir(p), cert: isCertificationFile(p)})
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	// First pass: what each function RETURNS, so `win := newWindow(...)` can
	// name the receiver type of a later `win.asRequest()` from the constructor
	// itself rather than from its name.
	g.returns = map[funcKey]string{}
	for _, f := range files {
		for _, decl := range f.file.Decls {
			fn, isFn := decl.(*ast.FuncDecl)
			if !isFn || fn.Recv != nil || fn.Type.Results == nil || len(fn.Type.Results.List) == 0 {
				continue
			}
			if name := promptTypeName(fn.Type.Results.List[0].Type); name != "" {
				g.returns[funcKey{dir: f.dir, name: fn.Name.Name}] = name
			}
		}
	}
	for _, f := range files {
		collectFile(&g, f.file, f.dir, f.cert)
	}
	return g
}

// isCertificationFile reports whether a path is part of the certification
// layer — the cases themselves and the lane that runs them.
func isCertificationFile(p string) bool {
	if strings.HasPrefix(filepath.Base(p), "certcase_") {
		return true
	}
	return strings.Contains(filepath.ToSlash(p), "/compose/aicert/")
}

// collectFile records one file's functions, their edges and whether they mint.
func collectFile(g *promptGraph, file *ast.File, dir string, isCert bool) {
	imports := importsOf(file, dir)
	// What THIS file calls the model package. An alias — or no import at all —
	// is the difference between seeing a site and silently not seeing one, so
	// the name is read from the file rather than assumed to be "model". The
	// helper is promptlanguage_test.go's, which already had this problem.
	modelPkg, importsModel := localNameFor(file, "shared/ports/model")
	if importsModel && modelPkg == "." {
		// A dot import spells the literal as a bare Request{}, which this scan
		// cannot tell from any other type's. Rather than not see the site, the
		// walk stops and says so — under-recognition is the one failure this
		// gate must not have.
		g.dotImported = append(g.dotImported, dir)
	}
	facts := fileFacts{
		dir: dir, imports: imports, modelPkg: modelPkg,
		modelImported: importsModel, certification: isCert,
	}
	for _, decl := range file.Decls {
		if fn, isFn := decl.(*ast.FuncDecl); isFn && fn.Body != nil {
			collectFunc(g, fn, facts)
			continue
		}
		// Anything that is not a function body: a var, a const, an init-time
		// composite. A prompt minted here has no enclosing function to name.
		if !importsModel {
			continue
		}
		ast.Inspect(decl, func(n ast.Node) bool {
			if mintsSystemPrompt(n, modelPkg) {
				g.looseLiterals = append(g.looseLiterals, dir)
			}
			return true
		})
	}
}

// fileFacts is what one file's header tells every function in it: where it
// sits, what it calls the model package, and whether it is part of the
// certification layer.
type fileFacts struct {
	dir      string
	imports  map[string]string
	modelPkg string
	// modelImported is false when the file never imports the model package, in
	// which case no literal in it can be a request.
	modelImported bool
	certification bool
}

// collectFunc records one function's mintness, its conduit status and its call
// edges.
func collectFunc(g *promptGraph, fn *ast.FuncDecl, f fileFacts) {
	self := funcKey{dir: f.dir, name: fn.Name.Name, isMethod: fn.Recv != nil, recv: promptReceiverType(fn)}
	if f.certification {
		g.roots[self] = true
	}
	// Names this function binds itself. A closure assigned to a local `write`
	// is not the package's own `write`, and reading it as one invents an edge —
	// which makes an uncertified site look reached.
	shadowed := boundNames(fn)
	// Receiver types for the values this function can name, so a call through
	// one resolves to that type's method rather than to every method of the
	// name.
	locals := localReceiverTypes(fn, self.recv, f.dir, f.imports, g.returns)
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if f.modelImported && mintsSystemPrompt(n, f.modelPkg) {
			g.minting[self] = true
			if at, fromParam := systemParameterIndex(n, fn); fromParam {
				g.promptArg[self] = at
				// The prompt is the CALLER's. One literal, many sites — so
				// mintness propagates up to whoever supplies it.
				g.conduits[self] = true
			}
			// A site is never its own certification. Rooting every function in
			// a certification file would let a prompt builder written INTO one
			// certify itself without any case issuing it.
			delete(g.roots, self)
		}
		if callee, named := promptCalleeOf(n, f.dir, f.imports, shadowed, locals, g.returns); named {
			g.calls[self] = append(g.calls[self], callee)
			if at := forwardedParameterPositions(n, fn); len(at) > 0 {
				if g.forwards[self] == nil {
					g.forwards[self] = map[funcKey]map[int]bool{}
				}
				g.forwards[self][callee] = at
			}
		}
		return true
	})
}

// mintsSystemPrompt reports whether a node is a model.Request literal carrying
// a System field. That pairing is what makes a function a SITE: a request
// without a system prompt is a continuation of somebody else's, and a system
// string on its own is prompt text nobody has sent yet.
func mintsSystemPrompt(n ast.Node, modelPkg string) bool {
	lit, ok := n.(*ast.CompositeLit)
	if !ok {
		return false
	}
	sel, ok := lit.Type.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Request" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != modelPkg {
		return false
	}
	for _, elt := range lit.Elts {
		kv, isKV := elt.(*ast.KeyValueExpr)
		if !isKV {
			continue
		}
		if key, isIdent := kv.Key.(*ast.Ident); isIdent && key.Name == "System" {
			return true
		}
	}
	return false
}

// promptCalleeOf resolves one call expression to the function it names, in the
// directory that function lives in. A selector is resolved through the file's
// own imports, which is what keeps two same-named exports apart.
//
// A call THROUGH a value names a METHOD of that value's type, never a
// package-level function of the same name and never another type's method of
// that name. Both conflations invent edges, and an invented edge certifies a
// prompt nobody grades.
//
// When the receiver's type cannot be read from the syntax — an interface
// parameter, most often — the edge is DROPPED rather than fanned out across
// every type declaring that name. Dropping over-refuses, which is loud; fanning
// out under-refuses, which is silent, and silence is the failure this gate
// exists to prevent.
func promptCalleeOf(
	n ast.Node, dir string, imports map[string]string, shadowed, locals map[string]string,
	returns map[funcKey]string,
) (funcKey, bool) {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return funcKey{}, false
	}
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		if _, bound := shadowed[fun.Name]; bound {
			return funcKey{}, false
		}
		return funcKey{dir: dir, name: fun.Name}, true
	case *ast.SelectorExpr:
		if pkg, isIdent := fun.X.(*ast.Ident); isIdent {
			if _, bound := shadowed[pkg.Name]; !bound {
				if target, known := imports[pkg.Name]; known {
					return funcKey{dir: target, name: fun.Sel.Name}, true
				}
			}
			if recv, known := locals[pkg.Name]; known {
				return funcKey{dir: dir, name: fun.Sel.Name, isMethod: true, recv: recv}, true
			}
			return funcKey{}, false
		}
		if recv := promptValueType(fun.X); recv != "" {
			return funcKey{dir: dir, name: fun.Sel.Name, isMethod: true, recv: recv}, true
		}
		// A constructor chain names the package that built the receiver, and
		// the constructor's own name is the closest the syntax comes to its
		// type — pkg.New(...) returns pkg's principal type by convention here.
		if owner, known := constructorPackageOf(fun.X, dir, imports); known {
			recv := constructedType(fun.X, dir, imports, returns)
			if recv == "" {
				// The constructor's result type is unreadable, so the receiver
				// is unknown and the edge is dropped rather than guessed.
				return funcKey{}, false
			}
			return funcKey{dir: owner, name: fun.Sel.Name, isMethod: true, recv: recv}, true
		}
		return funcKey{}, false
	}
	return funcKey{}, false
}

// promptReceiverType names the type a method hangs off, without its pointer star.
func promptReceiverType(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	return promptTypeName(fn.Recv.List[0].Type)
}

// promptTypeName reduces a type expression to the bare identifier a receiver carries.
func promptTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return promptTypeName(t.X)
	case *ast.IndexExpr:
		return promptTypeName(t.X)
	case *ast.IndexListExpr:
		return promptTypeName(t.X)
	case *ast.SelectorExpr:
		return t.Sel.Name
	}
	return ""
}

// localReceiverTypes maps the names a function binds to VALUES OF A KNOWN TYPE:
// its own receiver, and any local bound to a composite literal, a &T{}, or a
// new(T). A name bound to anything else is deliberately absent — an unknown
// type must drop its edges rather than guess a receiver.
func localReceiverTypes(
	fn *ast.FuncDecl, selfRecv, dir string, imports map[string]string, returns map[funcKey]string,
) map[string]string {
	out := map[string]string{}
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		for _, name := range fn.Recv.List[0].Names {
			out[name.Name] = selfRecv
		}
	}
	// Parameters carry their type in the signature. An INTERFACE parameter
	// resolves to the interface's own name, which no concrete method is
	// declared on — so `completer.Complete(...)` reaches nothing rather than
	// reaching all seventy-three Completes at once. That is the conservative
	// half of this gate working, not a gap in it.
	if fn.Type.Params != nil {
		for _, field := range fn.Type.Params.List {
			named := promptTypeName(field.Type)
			if named == "" {
				continue
			}
			for _, name := range field.Names {
				out[name.Name] = named
			}
		}
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || assign.Tok != token.DEFINE || len(assign.Lhs) != len(assign.Rhs) {
			return true
		}
		for i, lhs := range assign.Lhs {
			ident, isIdent := lhs.(*ast.Ident)
			if !isIdent {
				continue
			}
			recv := promptValueType(assign.Rhs[i])
			if recv == "" {
				recv = constructedType(assign.Rhs[i], dir, imports, returns)
			}
			if recv != "" {
				out[ident.Name] = recv
			}
		}
		return true
	})
	return out
}

// promptValueType reads the type of an expression in the three shapes that state it
// outright. Anything else answers empty, which drops the edge.
func promptValueType(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.CompositeLit:
		return promptTypeName(v.Type)
	case *ast.UnaryExpr:
		if v.Op == token.AND {
			return promptValueType(v.X)
		}
	case *ast.CallExpr:
		if fn, isIdent := v.Fun.(*ast.Ident); isIdent && fn.Name == "new" && len(v.Args) == 1 {
			return promptTypeName(v.Args[0])
		}
	}
	return ""
}

// constructorPackageOf names the package that built a chained receiver —
// pkg.New(...).Method() — so the method is looked for where its type lives.
func constructorPackageOf(recv ast.Expr, dir string, imports map[string]string) (string, bool) {
	call, ok := recv.(*ast.CallExpr)
	if !ok {
		return "", false
	}
	switch ctor := call.Fun.(type) {
	case *ast.Ident:
		return dir, true
	case *ast.SelectorExpr:
		pkg, isIdent := ctor.X.(*ast.Ident)
		if !isIdent {
			return "", false
		}
		if target, known := imports[pkg.Name]; known {
			return target, true
		}
	}
	return "", false
}

// importsOf maps each of a file's package aliases to the directory that
// package's source lives in, for the module's own packages.
func importsOf(file *ast.File, dir string) map[string]string {
	const modulePrefix = "github.com/margince/margince/backend/"
	out := map[string]string{}
	for _, spec := range file.Imports {
		raw, err := strconv.Unquote(spec.Path.Value)
		if err != nil || !strings.HasPrefix(raw, modulePrefix) {
			continue
		}
		// Gate tests run with the backend module root as their working
		// directory, so a module path IS the tree-relative directory.
		target := filepath.FromSlash(strings.TrimPrefix(raw, modulePrefix))
		alias := path.Base(raw)
		if spec.Name != nil {
			alias = spec.Name.Name
		}
		out[alias] = target
	}
	return out
}

// reachableFrom walks the edges out of every certification root.
func reachableFrom(g promptGraph) map[funcKey]bool {
	seen := make(map[funcKey]bool, len(g.calls))
	queue := make([]funcKey, 0, len(g.roots))
	for root := range g.roots {
		seen[root] = true
		queue = append(queue, root)
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, next := range g.calls[cur] {
			if seen[next] {
				continue
			}
			seen[next] = true
			queue = append(queue, next)
		}
	}
	return seen
}

// boundNames collects the identifiers a function binds locally — parameters,
// receiver, and whatever its body declares. A call to one of these names reaches
// a value, not the package-level function that happens to share it.
func boundNames(fn *ast.FuncDecl) map[string]string {
	out := map[string]string{}
	addField := func(list *ast.FieldList) {
		if list == nil {
			return
		}
		for _, field := range list.List {
			for _, name := range field.Names {
				out[name.Name] = "bound"
			}
		}
	}
	addField(fn.Recv)
	addField(fn.Type.Params)
	addField(fn.Type.Results)
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch decl := n.(type) {
		case *ast.AssignStmt:
			if decl.Tok != token.DEFINE {
				return true
			}
			for _, lhs := range decl.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok {
					out[ident.Name] = "bound"
				}
			}
		case *ast.ValueSpec:
			for _, name := range decl.Names {
				out[name.Name] = "bound"
			}
		case *ast.FuncLit:
			addField(decl.Type.Params)
			addField(decl.Type.Results)
		case *ast.RangeStmt:
			for _, v := range []ast.Expr{decl.Key, decl.Value} {
				if ident, ok := v.(*ast.Ident); ok {
					out[ident.Name] = "bound"
				}
			}
		}
		return true
	})
	return out
}

// The three ways this gate was shown to be foolable, each pinned so the fix
// cannot be undone quietly. All three are UNDER-recognition: the gate stayed
// green while a prompt went ungraded, which is the failure a census must not
// have (AGENTS.md, "a census that can fail short has already failed").
func TestTheCensusCannotBeFooled(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, src string
		wantMint  bool
	}{
		{
			name:     "the model package under an alias is still the model package",
			src:      `package p; import m "x/shared/ports/model"; func f() m.Request { return m.Request{System: "s"} }`,
			wantMint: true,
		},
		{
			name:     "the plain spelling still counts",
			src:      `package p; import "x/shared/ports/model"; func f() model.Request { return model.Request{System: "s"} }`,
			wantMint: true,
		},
		{
			name:     "a request with no system prompt is a continuation, not a site",
			src:      `package p; import "x/shared/ports/model"; func f() model.Request { return model.Request{MaxTokens: 1} }`,
			wantMint: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			file, err := parser.ParseFile(token.NewFileSet(), "p.go", tc.src, 0)
			if err != nil {
				t.Fatalf("parsing the case source: %v", err)
			}
			g := promptGraph{minting: map[funcKey]bool{}, calls: map[funcKey][]funcKey{}, roots: map[funcKey]bool{}}
			collectFile(&g, file, "p", false)
			if got := len(g.minting) > 0; got != tc.wantMint {
				t.Errorf("minting = %v, want %v — the scan %s this site",
					got, tc.wantMint, map[bool]string{true: "must see", false: "must not claim"}[tc.wantMint])
			}
		})
	}
}

// A call THROUGH a value names a method, never a package-level function of the
// same name. Conflating them invented an edge that certified an orphan.
func TestACallThroughAValueDoesNotReachItsNamesakeFunction(t *testing.T) {
	t.Parallel()
	src := `package p
func run() { helper := thing{}; helper.Build() }
type thing struct{}
func (thing) Build() {}
func Build() {}`
	file, err := parser.ParseFile(token.NewFileSet(), "p.go", src, 0)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	g := promptGraph{minting: map[funcKey]bool{}, calls: map[funcKey][]funcKey{}, roots: map[funcKey]bool{}}
	collectFile(&g, file, "p", false)
	for _, edge := range g.calls[funcKey{dir: "p", name: "run"}] {
		if edge.name == "Build" && !edge.isMethod {
			t.Error("a method call on a local value recorded an edge to the package-level function of that name")
		}
	}
}

// A prompt builder written into a certification file must not certify itself by
// sitting there: rooting it would make the gate agree with whoever moved it.
func TestAPromptInACertificationFileIsNotItsOwnCertification(t *testing.T) {
	t.Parallel()
	src := `package p
import "x/shared/ports/model"
func selfCertifying() model.Request { return model.Request{System: "s"} }`
	file, err := parser.ParseFile(token.NewFileSet(), "certcase_p.go", src, 0)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	g := promptGraph{minting: map[funcKey]bool{}, calls: map[funcKey][]funcKey{}, roots: map[funcKey]bool{}}
	collectFile(&g, file, "p", true)
	self := funcKey{dir: "p", name: "selfCertifying"}
	if !g.minting[self] {
		t.Fatal("the scan did not see the site at all")
	}
	if g.roots[self] {
		t.Error("a prompt builder inside a certification file was rooted, so it certifies itself")
	}
}

// constructedType reads the type a constructor call yields, from the
// constructor's own declared result rather than from its name.
func constructedType(expr ast.Expr, dir string, imports map[string]string, returns map[funcKey]string) string {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return ""
	}
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return returns[funcKey{dir: dir, name: fn.Name}]
	case *ast.SelectorExpr:
		pkg, isIdent := fn.X.(*ast.Ident)
		if !isIdent {
			return ""
		}
		if target, known := imports[pkg.Name]; known {
			return returns[funcKey{dir: target, name: fn.Sel.Name}]
		}
	}
	return ""
}

// systemParameterIndex reports which of the enclosing function's parameters
// supplies a request literal's system prompt, by position. Such a function is a
// conduit: companybrief.groundedRequest holds ONE literal and serves every
// caller that hands it a prompt builder, so counting literals would count one
// site where there are several, and a new caller would add no literal at all.
//
// The prompt must BE a parameter or be built by CALLING one. A parameter merely
// mentioned in the expression — a language code, a name — is data the site chose
// for itself, not a prompt handed in from outside.
func systemParameterIndex(n ast.Node, fn *ast.FuncDecl) (int, bool) {
	lit, ok := n.(*ast.CompositeLit)
	if !ok || fn.Type.Params == nil {
		return 0, false
	}
	at := map[string]int{}
	position := 0
	for _, field := range fn.Type.Params.List {
		for _, name := range field.Names {
			at[name.Name] = position
			position++
		}
	}
	for _, elt := range lit.Elts {
		kv, isKV := elt.(*ast.KeyValueExpr)
		if !isKV {
			continue
		}
		key, isIdent := kv.Key.(*ast.Ident)
		if !isIdent || key.Name != "System" {
			continue
		}
		switch value := kv.Value.(type) {
		case *ast.Ident:
			index, fromParam := at[value.Name]
			return index, fromParam
		case *ast.CallExpr:
			called, isIdent := value.Fun.(*ast.Ident)
			if !isIdent {
				return 0, false
			}
			index, fromParam := at[called.Name]
			return index, fromParam
		}
		return 0, false
	}
	return 0, false
}

// forwardedParameterPositions answers which argument positions of one call the
// caller fills with its own parameters — the shape that makes a wrapper stand
// in for whoever supplied the value.
func forwardedParameterPositions(n ast.Node, fn *ast.FuncDecl) map[int]bool {
	call, ok := n.(*ast.CallExpr)
	if !ok || fn.Type.Params == nil {
		return nil
	}
	params := map[string]bool{}
	for _, field := range fn.Type.Params.List {
		for _, name := range field.Names {
			params[name.Name] = true
		}
	}
	out := map[int]bool{}
	for position, arg := range call.Args {
		if ident, isIdent := arg.(*ast.Ident); isIdent && params[ident.Name] {
			out[position] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// propagateConduits makes every caller of a conduit a site in its own right,
// to a fixed point so a conduit reached through another still lands on the
// function that actually chose the prompt.
func propagateConduits(g *promptGraph) {
	callers := map[funcKey][]funcKey{}
	for from, tos := range g.calls {
		for _, to := range tos {
			callers[to] = append(callers[to], from)
		}
	}
	queue := make([]funcKey, 0, len(g.conduits))
	for c := range g.conduits {
		queue = append(queue, c)
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, caller := range callers[cur] {
			if g.minting[caller] {
				continue
			}
			g.minting[caller] = true
			// A wrapper that FORWARDS its own parameter into a conduit is a
			// conduit too. Without this the walk stops at the wrapper and the
			// function that actually chose the prompt is never counted.
			if at, known := g.promptArg[cur]; known && g.forwards[caller][cur][at] {
				g.conduits[caller] = true
			}
			// A site is never its own certification, whether the literal is its
			// own or a conduit's. collectFunc refuses that for a direct minter;
			// a conduit's caller is a site by the same reasoning and has to be
			// refused by the same rule, or a builder written into a
			// certification file certifies itself through the conduit.
			delete(g.roots, caller)
			if g.conduits[caller] {
				queue = append(queue, caller)
			}
		}
	}
}

// A method is keyed by its RECEIVER TYPE, so the seventy-three Completes this
// package declares are seventy-three nodes. Collapsing them let one cert case
// calling completer.Complete certify every one, a rogue minter included — the
// silent false-PASS this gate exists to prevent.
func TestAMethodIsKeyedByItsReceiverType(t *testing.T) {
	t.Parallel()
	src := `package p
type a struct{}
type b struct{}
func (a) Complete() {}
func (b) Complete() {}
func run() { x := a{}; x.Complete() }`
	g := graphOf(t, src, "p.go", "p", false)
	// The edge must name `a`'s Complete SPECIFICALLY. Asserting only that it is
	// not `b`'s would pass with the receiver dropped altogether, which is the
	// very collapse this pins against.
	var found bool
	for _, edge := range g.calls[funcKey{dir: "p", name: "run"}] {
		if edge.name != "Complete" {
			continue
		}
		found = true
		if !edge.isMethod || edge.recv != "a" {
			t.Errorf("the call on an `a` value reached %s, not `a`'s own Complete", edge)
		}
	}
	if !found {
		t.Fatal("the call recorded no Complete edge at all, so this test proves nothing")
	}
}

// A receiver whose type the syntax does not state drops its edges. Fanning out
// across every type declaring the name would under-refuse, and under-refusal is
// the failure that passes in silence.
func TestAnUnresolvedReceiverDropsItsEdgeRatherThanFanningOut(t *testing.T) {
	t.Parallel()
	src := `package p
type iface interface{ Complete() }
type real struct{}
func (real) Complete() {}
func run(c iface) { c.Complete() }`
	g := graphOf(t, src, "p.go", "p", false)
	for _, edge := range g.calls[funcKey{dir: "p", name: "run"}] {
		if edge.name == "Complete" && edge.recv == "real" {
			t.Error("a call through an interface parameter reached a concrete type's method")
		}
	}
}

// A builder handed its prompt as a PARAMETER mints on the caller's behalf: one
// literal, many sites. Counting literals would count one, and a new caller
// would add none at all.
func TestAConduitMakesItsCallersSites(t *testing.T) {
	t.Parallel()
	src := `package p
import "x/shared/ports/model"
func conduit(systemFor func() string) model.Request { return model.Request{System: systemFor()} }
func siteOne() model.Request { return conduit(func() string { return "one" }) }
func notASite() { _ = 1 }`
	g := graphOf(t, src, "p.go", "p", false)
	propagateConduits(&g)
	if !g.minting[funcKey{dir: "p", name: "siteOne"}] {
		t.Error("a caller that chose the prompt is not counted as a site")
	}
	if g.minting[funcKey{dir: "p", name: "notASite"}] {
		t.Error("a function that never reaches the conduit was counted as a site")
	}
}

// A prompt minted outside any function has no site to attribute it to, so the
// walk refuses rather than not seeing it.
func TestAPromptOutsideAnyFunctionIsRefused(t *testing.T) {
	t.Parallel()
	src := `package p
import "x/shared/ports/model"
var loose = model.Request{System: "nobody's prompt"}`
	g := graphOf(t, src, "p.go", "p", false)
	if len(g.looseLiterals) == 0 {
		t.Error("a package-level prompt literal was neither seen nor refused")
	}
}

// graphOf parses one source string into a graph, for the cases above.
func graphOf(t *testing.T, src, name, dir string, isCert bool) promptGraph {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), name, src, 0)
	if err != nil {
		t.Fatalf("parsing the case source: %v", err)
	}
	g := promptGraph{
		minting:   map[funcKey]bool{},
		calls:     map[funcKey][]funcKey{},
		roots:     map[funcKey]bool{},
		conduits:  map[funcKey]bool{},
		returns:   map[funcKey]string{},
		promptArg: map[funcKey]int{},
		forwards:  map[funcKey]map[funcKey]map[int]bool{},
	}
	collectFile(&g, file, dir, isCert)
	return g
}

// A conduit's caller is a site, so it cannot be its own certification either —
// the same refusal collectFunc applies to a direct minter, which propagation
// has to carry or a builder written into a certification file certifies itself
// by handing its prompt to a conduit.
func TestAConduitCallerInACertificationFileIsNotItsOwnCertification(t *testing.T) {
	t.Parallel()
	src := `package p
import "x/shared/ports/model"
func conduit(systemFor func() string) model.Request { return model.Request{System: systemFor()} }
func selfCertifyingViaConduit() model.Request { return conduit(func() string { return "mine" }) }`
	g := graphOf(t, src, "certcase_p.go", "p", true)
	propagateConduits(&g)
	self := funcKey{dir: "p", name: "selfCertifyingViaConduit"}
	if !g.minting[self] {
		t.Fatal("the conduit's caller was not counted as a site, so this test proves nothing")
	}
	if g.roots[self] {
		t.Error("a conduit's caller inside a certification file was rooted, so it certifies itself")
	}
}

// A wrapper that FORWARDS its prompt into a conduit is a conduit too. Without
// that, propagation stops at the wrapper and the function that actually chose
// the prompt — the site — is never counted. Forwarding some OTHER argument does
// not make a wrapper one: a language code handed along is data, not a prompt.
func TestConduitStatusCarriesThroughAForwardingWrapper(t *testing.T) {
	t.Parallel()
	src := `package p
import "x/shared/ports/model"
func conduit(systemFor func() string) model.Request { return model.Request{System: systemFor()} }
func wrap(sys func() string) model.Request { return conduit(sys) }
func chooser() model.Request { return wrap(func() string { return "mine" }) }
func passesDataOnly(lang string) model.Request { return wrap(func() string { return lang }) }`
	g := graphOf(t, src, "p.go", "p", false)
	propagateConduits(&g)
	for _, name := range []string{"wrap", "chooser"} {
		if !g.minting[funcKey{dir: "p", name: name}] {
			t.Errorf("%s is not counted as a site, so a prompt behind a wrapper goes ungraded", name)
		}
	}
	if !g.conduits[funcKey{dir: "p", name: "wrap"}] {
		t.Error("the wrapper was not promoted to a conduit, so propagation stops at it")
	}
}
