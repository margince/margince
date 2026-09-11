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
// builds a model.Request composite literal carrying a System field. Nothing
// else in the tree does that, and a new site cannot avoid doing it.
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
}

func (k funcKey) String() string { return k.dir + "." + k.name }

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
		minting: map[funcKey]bool{},
		calls:   map[funcKey][]funcKey{},
		roots:   map[funcKey]bool{},
	}
	fset := token.NewFileSet()
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
		collectFile(&g, file, filepath.Dir(p), isCertificationFile(p))
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
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
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		self := funcKey{dir: dir, name: fn.Name.Name}
		if isCert {
			g.roots[self] = true
		}
		// Names this function binds itself. A closure assigned to a local
		// `write` is not the package's own `write`, and reading it as one
		// invents an edge — which makes an uncertified site look reached.
		shadowed := boundNames(fn)
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if mintsSystemPrompt(n) {
				g.minting[self] = true
			}
			if callee, named := promptCalleeOf(n, dir, imports, shadowed); named {
				g.calls[self] = append(g.calls[self], callee)
			}
			return true
		})
	}
}

// mintsSystemPrompt reports whether a node is a model.Request literal carrying
// a System field. That pairing is what makes a function a SITE: a request
// without a system prompt is a continuation of somebody else's, and a system
// string on its own is prompt text nobody has sent yet.
func mintsSystemPrompt(n ast.Node) bool {
	lit, ok := n.(*ast.CompositeLit)
	if !ok {
		return false
	}
	sel, ok := lit.Type.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Request" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "model" {
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
func promptCalleeOf(n ast.Node, dir string, imports map[string]string, shadowed map[string]bool) (funcKey, bool) {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return funcKey{}, false
	}
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		if shadowed[fun.Name] {
			return funcKey{}, false
		}
		return funcKey{dir: dir, name: fun.Name}, true
	case *ast.SelectorExpr:
		pkg, isIdent := fun.X.(*ast.Ident)
		if !isIdent {
			// A method on an expression rather than a name. The one shape that
			// carries a resolvable package is the constructor chain the cert
			// cases use — runner.New(...).Run(...) — so the method belongs to
			// whatever package built the receiver.
			if owner, known := constructorPackage(fun.X, dir, imports); known {
				return funcKey{dir: owner, name: fun.Sel.Name}, true
			}
			return funcKey{dir: dir, name: fun.Sel.Name}, true
		}
		if target, known := imports[pkg.Name]; known && !shadowed[pkg.Name] {
			return funcKey{dir: target, name: fun.Sel.Name}, true
		}
		// An unknown qualifier is a local variable, not a package. Treat the
		// selected name as same-package rather than dropping the edge: a
		// missing edge is under-recognition, which is the failure this gate
		// must not have.
		return funcKey{dir: dir, name: fun.Sel.Name}, true
	}
	return funcKey{}, false
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
func boundNames(fn *ast.FuncDecl) map[string]bool {
	out := map[string]bool{}
	addField := func(list *ast.FieldList) {
		if list == nil {
			return
		}
		for _, field := range list.List {
			for _, name := range field.Names {
				out[name.Name] = true
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
					out[ident.Name] = true
				}
			}
		case *ast.ValueSpec:
			for _, name := range decl.Names {
				out[name.Name] = true
			}
		case *ast.FuncLit:
			addField(decl.Type.Params)
			addField(decl.Type.Results)
		case *ast.RangeStmt:
			for _, v := range []ast.Expr{decl.Key, decl.Value} {
				if ident, ok := v.(*ast.Ident); ok {
					out[ident.Name] = true
				}
			}
		}
		return true
	})
	return out
}

// constructorPackage answers which package built the receiver of a chained
// method call, for the one receiver shape that names one: a call to a
// constructor, qualified or local. Without it runner.New(...).Run(...) reads as
// a call to the CALLER's own Run, the edge into the runner is lost, and every
// site behind it reports as uncertified.
func constructorPackage(recv ast.Expr, dir string, imports map[string]string) (string, bool) {
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
