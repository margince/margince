// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H1

package gates

// An approval's summary is written through a per-language copy table, never as
// an English sentence in product code.
//
// The summary is stored once on the approval row and read by every seat that
// may decide it, so it follows the installation's base language, resolved when
// the row is written. A sentence typed inline is English on a German
// installation forever, and nothing about it looks wrong in review: the first
// approval kinds all wrote theirs that way, and each new kind copied the last.
//
// WHAT IT SEES. Every `Summary` handed to an approvals staging input — a
// composite literal key, an elided element of a slice of them, or an
// `x.Summary = …` assignment — anywhere under internal/. The input types are
// DERIVED, not listed: the seed is each struct the approvals Service's exported
// methods take that carries a Summary field, and a type joins whenever a known
// input's Summary is forwarded from its Summary (`Summary: in.Summary` in an
// adapter), which is how automation's and agents' own request shapes are found.
// From each site the value is followed through `+`, fmt calls, locals,
// same-package helpers (their return arms, with parameters bound to the call's
// arguments), a parameter's callers — any module's, for an exported seam
// method — and string constants. A literal
// reached that way is prose when, with format verbs stripped, it holds two
// words, or one word that is capitalised or sits beside a space ("Archive %s").
//
// WHAT PASSES, AND WHY. A selector off a local or parameter (`said.ghosted`,
// `in.Summary`, `item.Headline`) — a copy table's field or a forwarded value. A
// forwarded value is not product-written: model-written text answers to
// promptlanguage_test.go, and a headline or subject is quoted, not written. A
// map indexed by textlang.Lang is a copy table. A bare lowercase token ("offer")
// is an identifier, which passes through every language untranslated.
//
// WHAT IT CANNOT SEE. A helper reached as a function VALUE rather than a call;
// a method told apart from a same-named, same-arity one only by its receiver's
// type; a bare `return` of named results; a range variable's type; a module
// outside internal/; and anything more than approvalSummaryHops hops away.
// Nor a field another module fills and compose forwards: the selector reads as
// forwarded, so the writing module's own census holds that sentence (deals:
// TestEveryShippedLanguageWritesItsOwnDealSentences).

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// approvalsServiceDir is where the staging inputs are seeded from: the struct
// types its Service's exported methods accept.
const approvalsServiceDir = "internal/modules/approvals"

// The census never finds fewer staging-input Summary sites or input types than
// these. A renamed input struct or staging method would otherwise shrink it to
// nothing and read clean; merging sites lowers the floor in the same change.
const (
	approvalSummaryFloor = 72
	approvalInputFloor   = 6
)

// approvalSummaryHops bounds how far a value is followed from its site. Deep
// enough for site → helper → helper → local → literal with room to spare; the
// bound exists so a recursive helper cannot hang the gate.
const approvalSummaryHops = 8

func TestNoApprovalSummaryIsWrittenAsEnglishProse(t *testing.T) {
	t.Parallel()
	tree := loadSummaryTree(t, "internal")
	inputs := tree.stagingInputs(t)
	sites := tree.summarySites(inputs)
	if len(sites) < approvalSummaryFloor || len(inputs) < approvalInputFloor {
		t.Fatalf("found %d approval Summary sites across %d staging input types, below the floor of %d across %d: the "+
			"input structs or the approvals Service's staging methods moved, and the census shrank rather than "+
			"failing. Inputs derived: %v", len(sites), len(inputs), approvalSummaryFloor, approvalInputFloor, sortedKeys(inputs))
	}
	for _, site := range sites {
		for _, prose := range tree.proseReaching(site) {
			t.Errorf("%s: this approval's summary reaches English written in product code at %s (%q). The "+
				"summary is stored once and read by every seat, so it follows the installation's base language: "+
				"write the sentence into a per-language copy table (one entry per textlang.Shipped language) and "+
				"pick the entry with the base language resolved at write time",
				site.where, prose.where, prose.text)
		}
	}
}

// The walk is planted with one of every shape it claims to see and every shape
// it claims to pass, so a change that narrows it fails here rather than
// reporting a smaller tree as clean.
func TestApprovalSummaryCensusSeesEveryShapeItClaims(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	planted, err := os.ReadFile("gates/testdata/approvalsummarycopy.go.txt")
	if err != nil {
		t.Fatalf("reading the planted package: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "planted.go"), planted, 0o600); err != nil {
		t.Fatalf("writing the planted package: %v", err)
	}
	tree := loadSummaryTree(t, dir)
	inputs := map[string]bool{filepath.ToSlash(dir) + ".stageInput": true}
	flagged := map[string]bool{}
	passed := map[string]bool{}
	for _, site := range tree.summarySites(inputs) {
		if len(tree.proseReaching(site)) > 0 {
			flagged[site.function] = true
		} else {
			passed[site.function] = true
		}
	}
	writesEnglish := []string{
		"direct", "concat", "sprintf", "viaHelper", "viaSwitch", "viaLocal",
		"viaParam", "viaConst", "viaAssign", "viaOneWord", "viaTable",
	}
	for _, name := range writesEnglish {
		if !flagged[name] {
			t.Errorf("the planted %s site writes English and the census did not see it", name)
		}
	}
	for _, name := range []string{"formatOnly", "forwarded", "copyField", "langTable", "identifier"} {
		if !passed[name] {
			t.Errorf("the planted %s site writes no English and the census reported it (or never found it)", name)
		}
	}
}

// summarySite is one place a staging input's Summary is set.
type summarySite struct {
	where    string
	function string
	value    ast.Expr
	scope    *traceScope
}

// proseFinding is one English literal a site's value reaches.
type proseFinding struct {
	where string
	text  string
}

// summaryTree holds the hand-written packages under one root, parsed once.
type summaryTree struct {
	fset *token.FileSet
	pkgs map[string]*summaryPackage
}

// summaryPackage indexes one directory's declarations by name, which is as far
// as a syntactic census can resolve a call without a type checker.
type summaryPackage struct {
	dir     string
	files   []*ast.File
	bodies  map[string][]funcIn
	results map[string][]resultsIn
	vars    map[string]varIn
	structs map[string]*ast.StructType
	calls   []callIn
	consts  map[string]string
}

type varIn struct {
	value ast.Expr
	file  *ast.File
}

type funcIn struct {
	decl *ast.FuncDecl
	file *ast.File
}

// resultsIn is a declared signature's result list, from a func or an interface
// method, with the file whose imports its type names resolve against.
type resultsIn struct {
	results *ast.FieldList
	file    *ast.File
}

type callIn struct {
	call *ast.CallExpr
	file *ast.File
	fn   *ast.FuncDecl
}

func loadSummaryTree(t *testing.T, root string) *summaryTree {
	t.Helper()
	tree := &summaryTree{fset: token.NewFileSet(), pkgs: map[string]*summaryPackage{}}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") ||
			strings.HasSuffix(p, "_gen.go") || slices.Contains(strings.Split(filepath.ToSlash(p), "/"), "testdata") {
			return nil
		}
		file, parseErr := parser.ParseFile(tree.fset, p, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			return parseErr
		}
		dir := filepath.ToSlash(filepath.Dir(p))
		pkg := tree.pkgs[dir]
		if pkg == nil {
			pkg = newSummaryPackage(dir)
			tree.pkgs[dir] = pkg
		}
		pkg.index(file)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	for _, pkg := range tree.pkgs {
		pkg.consts = gatekit.PackageStringConstants(t, pkg.dir)
	}
	return tree
}

func newSummaryPackage(dir string) *summaryPackage {
	return &summaryPackage{
		dir: dir, bodies: map[string][]funcIn{}, results: map[string][]resultsIn{},
		vars: map[string]varIn{}, structs: map[string]*ast.StructType{},
	}
}

func (p *summaryPackage) index(file *ast.File) {
	p.files = append(p.files, file)
	for _, decl := range file.Decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			p.bodies[decl.Name.Name] = append(p.bodies[decl.Name.Name], funcIn{decl, file})
			p.results[decl.Name.Name] = append(p.results[decl.Name.Name], resultsIn{decl.Type.Results, file})
			ast.Inspect(decl, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					p.calls = append(p.calls, callIn{call, file, decl})
				}
				return true
			})
		case *ast.GenDecl:
			p.indexGeneral(decl, file)
		}
	}
}

func (p *summaryPackage) indexGeneral(decl *ast.GenDecl, file *ast.File) {
	for _, spec := range decl.Specs {
		switch spec := spec.(type) {
		case *ast.ValueSpec:
			if decl.Tok == token.VAR && len(spec.Names) == len(spec.Values) {
				for i, name := range spec.Names {
					p.vars[name.Name] = varIn{spec.Values[i], file}
				}
			}
		case *ast.TypeSpec:
			switch shape := spec.Type.(type) {
			case *ast.StructType:
				p.structs[spec.Name.Name] = shape
			case *ast.InterfaceType:
				for _, method := range shape.Methods.List {
					if sig, ok := method.Type.(*ast.FuncType); ok && len(method.Names) == 1 {
						name := method.Names[0].Name
						p.results[name] = append(p.results[name], resultsIn{sig.Results, file})
					}
				}
			}
		}
	}
}

// typeKey names a type expression as "<dir>.<Name>", resolving a package
// qualifier through the file's own imports so an alias still names the type.
func typeKey(expr ast.Expr, file *ast.File, dir string) string {
	switch expr := expr.(type) {
	case *ast.StarExpr:
		return typeKey(expr.X, file, dir)
	case *ast.Ident:
		return dir + "." + expr.Name
	case *ast.SelectorExpr:
		if target, ok := importedDir(file, expr.X); ok {
			return target + "." + expr.Sel.Name
		}
	}
	return ""
}

// importedDir resolves a package qualifier to the module-relative directory it
// imports, and reports false for anything that is not an in-module import.
func importedDir(file *ast.File, qualifier ast.Expr) (string, bool) {
	imported, ok := importedPath(file, qualifier)
	if !ok {
		return "", false
	}
	return strings.CutPrefix(imported, modulePath+"/")
}

// importedPath is the import path a qualifier names in this file, honouring an
// alias; false when the qualifier is not an import at all.
func importedPath(file *ast.File, qualifier ast.Expr) (string, bool) {
	ident, ok := qualifier.(*ast.Ident)
	if !ok {
		return "", false
	}
	for _, imp := range file.Imports {
		imported, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		local := path.Base(imported)
		if imp.Name != nil {
			local = imp.Name.Name
		}
		if local == ident.Name {
			return imported, true
		}
	}
	return "", false
}

func (tree *summaryTree) hasSummaryField(key string) bool {
	dot := strings.LastIndex(key, ".")
	if dot < 0 {
		return false
	}
	pkg := tree.pkgs[key[:dot]]
	if pkg == nil || pkg.structs[key[dot+1:]] == nil {
		return false
	}
	for _, field := range pkg.structs[key[dot+1:]].Fields.List {
		for _, name := range field.Names {
			if name.Name == "Summary" {
				return true
			}
		}
	}
	return false
}

// stagingInputs derives the staging input types: seeded from the approvals
// Service's exported methods, then closed over adapters that forward one
// input's Summary into another's.
func (tree *summaryTree) stagingInputs(t *testing.T) map[string]bool {
	t.Helper()
	inputs := map[string]bool{}
	approvals := tree.pkgs[approvalsServiceDir]
	if approvals == nil {
		t.Fatalf("%s holds no package, so the staging inputs cannot be seeded from it", approvalsServiceDir)
	}
	for _, funcs := range approvals.bodies {
		for _, fn := range funcs {
			if fn.decl.Recv == nil || !fn.decl.Name.IsExported() ||
				typeKey(fn.decl.Recv.List[0].Type, fn.file, approvals.dir) != approvals.dir+".Service" {
				continue
			}
			for _, param := range fn.decl.Type.Params.List {
				if key := typeKey(param.Type, fn.file, approvals.dir); tree.hasSummaryField(key) {
					inputs[key] = true
				}
			}
		}
	}
	if len(inputs) == 0 {
		t.Fatalf("no exported approvals Service method takes a struct with a Summary field, so the census has " +
			"no staging input to start from")
	}
	for grew := true; grew; {
		grew = false
		for _, site := range tree.summarySites(inputs) {
			forwarded, ok := site.value.(*ast.SelectorExpr)
			if !ok || forwarded.Sel.Name != "Summary" {
				continue
			}
			from, isIdent := forwarded.X.(*ast.Ident)
			if !isIdent {
				continue
			}
			if key := tree.localType(site.scope, from.Name); key != "" && !inputs[key] && tree.hasSummaryField(key) {
				inputs[key] = true
				grew = true
			}
		}
	}
	return inputs
}

// summarySites lists every Summary set on one of the staging inputs.
func (tree *summaryTree) summarySites(inputs map[string]bool) []summarySite {
	var sites []summarySite
	for _, pkg := range sortedPackages(tree.pkgs) {
		for _, file := range pkg.files {
			for _, decl := range file.Decls {
				fn, _ := decl.(*ast.FuncDecl)
				scope := &traceScope{pkg: pkg, file: file, fn: fn}
				sites = append(sites, tree.sitesIn(decl, scope, inputs)...)
			}
		}
	}
	return sites
}

func (tree *summaryTree) sitesIn(decl ast.Decl, scope *traceScope, inputs map[string]bool) []summarySite {
	var sites []summarySite
	add := func(key ast.Node, value ast.Expr) {
		name := ""
		if scope.fn != nil {
			name = scope.fn.Name.Name
		}
		sites = append(sites, summarySite{where: tree.position(key.Pos()), function: name, value: value, scope: scope})
	}
	ast.Inspect(decl, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CompositeLit:
			if inputs[typeKey(n.Type, scope.file, scope.pkg.dir)] {
				summaryKeys(n, add)
			}
			if elem := elementType(n.Type); elem != nil && inputs[typeKey(elem, scope.file, scope.pkg.dir)] {
				for _, elt := range n.Elts {
					if kv, ok := elt.(*ast.KeyValueExpr); ok {
						elt = kv.Value
					}
					if lit, ok := elt.(*ast.CompositeLit); ok && lit.Type == nil {
						summaryKeys(lit, add)
					}
				}
			}
		case *ast.AssignStmt:
			for i, lhs := range n.Lhs {
				sel, ok := lhs.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "Summary" || len(n.Rhs) != len(n.Lhs) {
					continue
				}
				if holder, ok := sel.X.(*ast.Ident); ok && inputs[tree.localType(scope, holder.Name)] {
					add(sel, n.Rhs[i])
				}
			}
		}
		return true
	})
	return sites
}

func summaryKeys(lit *ast.CompositeLit, add func(ast.Node, ast.Expr)) {
	for _, elt := range lit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "Summary" {
				add(kv, kv.Value)
			}
		}
	}
}

// elementType is the element of a slice, array or map literal's type, whose
// elements may elide it.
func elementType(expr ast.Expr) ast.Expr {
	switch expr := expr.(type) {
	case *ast.ArrayType:
		return expr.Elt
	case *ast.MapType:
		return expr.Value
	}
	return nil
}

// localType resolves the declared type of a parameter, receiver or local in
// the scope's function: from its declaration, the composite literal that
// initialises it, or the result of the same-package call that does.
func (tree *summaryTree) localType(scope *traceScope, name string) string {
	fn := scope.fn
	if fn == nil {
		return ""
	}
	for _, fields := range []*ast.FieldList{fn.Recv, fn.Type.Params} {
		if fields == nil {
			continue
		}
		for _, field := range fields.List {
			for _, ident := range field.Names {
				if ident.Name == name {
					return typeKey(field.Type, scope.file, scope.pkg.dir)
				}
			}
		}
	}
	found := ""
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if found != "" {
			return false
		}
		switch n := n.(type) {
		case *ast.AssignStmt:
			if n.Tok == token.DEFINE {
				found = tree.definedType(scope, n.Lhs, n.Rhs, name)
			}
		case *ast.ValueSpec:
			for i, ident := range n.Names {
				if ident.Name != name {
					continue
				}
				if n.Type != nil {
					found = typeKey(n.Type, scope.file, scope.pkg.dir)
				} else if len(n.Values) == len(n.Names) {
					found = tree.exprType(scope, n.Values[i], 0)
				}
			}
		}
		return true
	})
	return found
}

func (tree *summaryTree) definedType(scope *traceScope, lhs, rhs []ast.Expr, name string) string {
	for i, target := range lhs {
		ident, ok := target.(*ast.Ident)
		if !ok || ident.Name != name {
			continue
		}
		if len(rhs) == len(lhs) {
			return tree.exprType(scope, rhs[i], 0)
		}
		if len(rhs) == 1 {
			return tree.exprType(scope, rhs[0], i)
		}
	}
	return ""
}

func (tree *summaryTree) exprType(scope *traceScope, expr ast.Expr, index int) string {
	switch expr := expr.(type) {
	case *ast.CompositeLit:
		return typeKey(expr.Type, scope.file, scope.pkg.dir)
	case *ast.UnaryExpr:
		return tree.exprType(scope, expr.X, index)
	case *ast.CallExpr:
		agreed := ""
		for _, sig := range scope.pkg.results[calleeName(expr)] {
			if sig.results == nil || calleeName(expr) == "" || isQualified(scope.file, expr.Fun) {
				continue
			}
			key := typeKey(resultType(sig.results, index), sig.file, scope.pkg.dir)
			if agreed != "" && key != agreed {
				return ""
			}
			agreed = key
		}
		return agreed
	}
	return ""
}

func resultType(results *ast.FieldList, index int) ast.Expr {
	at := 0
	for _, field := range results.List {
		width := max(1, len(field.Names))
		if index < at+width {
			return field.Type
		}
		at += width
	}
	return nil
}

// isQualified reports whether a call's function is reached through an import,
// which names another package's function rather than this one's.
func isQualified(file *ast.File, fun ast.Expr) bool {
	sel, ok := fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	_, imported := importedPath(file, sel.X)
	return imported
}

func (tree *summaryTree) position(pos token.Pos) string {
	at := tree.fset.Position(pos)
	return fmt.Sprintf("%s:%d", filepath.ToSlash(at.Filename), at.Line)
}

func sortedPackages(pkgs map[string]*summaryPackage) []*summaryPackage {
	out := make([]*summaryPackage, 0, len(pkgs))
	for _, pkg := range pkgs {
		out = append(out, pkg)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].dir < out[j].dir })
	return out
}

// traceScope is where an expression is read: the function around it, and the
// arguments its parameters are bound to when a call led the walk there.
type traceScope struct {
	pkg   *summaryPackage
	file  *ast.File
	fn    *ast.FuncDecl
	bound map[string]boundArg
}

type boundArg struct {
	expr  ast.Expr
	scope *traceScope
}

// proseTrace follows one site's value to every literal it can reach.
type proseTrace struct {
	tree    *summaryTree
	visited map[visitKey]bool
	found   map[string]proseFinding
}

// visitKey stops a name that feeds itself (`s += "…"`) from being followed
// round its own loop.
type visitKey struct {
	name  string
	scope *traceScope
}

func (tree *summaryTree) proseReaching(site summarySite) []proseFinding {
	trace := &proseTrace{tree: tree, visited: map[visitKey]bool{}, found: map[string]proseFinding{}}
	trace.expr(site.value, site.scope, 0)
	out := make([]proseFinding, 0, len(trace.found))
	for _, finding := range trace.found {
		out = append(out, finding)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].where < out[j].where })
	return out
}

func (tr *proseTrace) expr(expr ast.Expr, scope *traceScope, hops int) {
	if hops > approvalSummaryHops {
		return
	}
	switch expr := expr.(type) {
	case *ast.BasicLit:
		if text, ok := gatekit.StringExpr(expr, nil, gatekit.FoldStrict); ok {
			tr.judge(expr.Pos(), text)
		}
	case *ast.ParenExpr:
		tr.expr(expr.X, scope, hops)
	case *ast.StarExpr:
		tr.expr(expr.X, scope, hops)
	case *ast.UnaryExpr:
		tr.expr(expr.X, scope, hops)
	case *ast.BinaryExpr:
		tr.concat(expr, scope, hops)
	case *ast.CallExpr:
		tr.call(expr, scope, hops, 0)
	case *ast.Ident:
		tr.ident(expr, scope, hops)
	case *ast.SelectorExpr:
		tr.selector(expr, scope, hops)
	case *ast.IndexExpr:
		tr.index(expr, scope, hops)
	}
}

func (tr *proseTrace) judge(pos token.Pos, text string) {
	if isEnglishProse(text) {
		where := tr.tree.position(pos)
		tr.found[where] = proseFinding{where: where, text: text}
	}
}

// concat judges a `+` chain as the one sentence it builds, since "Rename " and
// " to " are single words apart and a sentence together, and follows every
// operand that is not a literal.
func (tr *proseTrace) concat(expr *ast.BinaryExpr, scope *traceScope, hops int) {
	if expr.Op != token.ADD {
		return
	}
	var sentence strings.Builder
	first := expr.Pos()
	var walk func(ast.Expr)
	walk = func(operand ast.Expr) {
		if paren, ok := operand.(*ast.ParenExpr); ok {
			operand = paren.X
		}
		if inner, ok := operand.(*ast.BinaryExpr); ok && inner.Op == token.ADD {
			walk(inner.X)
			walk(inner.Y)
			return
		}
		if text, ok := gatekit.StringExpr(operand, scope.pkg.consts, gatekit.FoldStrict); ok && !tr.declared(scope, operand) {
			sentence.WriteString(text)
			return
		}
		sentence.WriteString(gatekit.ComputedFragment)
		tr.expr(operand, scope, hops)
	}
	walk(expr)
	tr.judge(first, sentence.String())
}

// declared reports whether an identifier operand names a parameter or local,
// which shadows a package constant of the same name.
func (tr *proseTrace) declared(scope *traceScope, operand ast.Expr) bool {
	ident, ok := operand.(*ast.Ident)
	return ok && (scope.bound[ident.Name].expr != nil || summaryParamIndex(scope.fn, ident.Name) >= 0 ||
		len(localAssignments(scope.fn, ident.Name)) > 0)
}

// call follows a same-package function into its return arms, and reads every
// argument of anything else — fmt.Sprintf's format and operands included.
func (tr *proseTrace) call(call *ast.CallExpr, scope *traceScope, hops, index int) {
	callees := tr.callees(call, scope)
	if len(callees) == 0 {
		for _, arg := range call.Args {
			tr.expr(arg, scope, hops)
		}
		return
	}
	for _, callee := range callees {
		inner := &traceScope{pkg: scope.pkg, file: callee.file, fn: callee.decl, bound: map[string]boundArg{}}
		at := 0
		for _, field := range callee.decl.Type.Params.List {
			for _, name := range field.Names {
				if at < len(call.Args) {
					inner.bound[name.Name] = boundArg{call.Args[at], scope}
				}
				at++
			}
		}
		ast.Inspect(callee.decl.Body, func(n ast.Node) bool {
			if _, isClosure := n.(*ast.FuncLit); isClosure {
				return false
			}
			if ret, ok := n.(*ast.ReturnStmt); ok && index < len(ret.Results) && (index == 0 || len(ret.Results) > 1) {
				tr.expr(ret.Results[index], inner, hops+1)
			}
			return true
		})
	}
}

// callees are the same-package declarations a call may reach: a bare name is a
// function, a selector not through an import is a method of that name.
func (tr *proseTrace) callees(call *ast.CallExpr, scope *traceScope) []funcIn {
	name := calleeName(call)
	if name == "" || isQualified(scope.file, call.Fun) {
		return nil
	}
	_, method := call.Fun.(*ast.SelectorExpr)
	var out []funcIn
	for _, fn := range scope.pkg.bodies[name] {
		if fn.decl.Body != nil && (fn.decl.Recv != nil) == method {
			out = append(out, fn)
		}
	}
	return out
}

// ident follows a name to whatever gives it a value: the argument bound to it,
// the in-package callers of the function it is a parameter of, its local
// assignments, or a package constant or variable.
func (tr *proseTrace) ident(ident *ast.Ident, scope *traceScope, hops int) {
	key := visitKey{ident.Name, scope}
	if tr.visited[key] {
		return
	}
	tr.visited[key] = true
	local := false
	if arg, ok := scope.bound[ident.Name]; ok {
		tr.expr(arg.expr, arg.scope, hops+1)
		local = true
	} else if at := summaryParamIndex(scope.fn, ident.Name); at >= 0 {
		tr.callers(scope, at, hops+1)
		local = true
	}
	for _, assigned := range localAssignments(scope.fn, ident.Name) {
		local = true
		if assigned.call != nil {
			tr.call(assigned.call, scope, hops+1, assigned.index)
		} else {
			tr.expr(assigned.value, scope, hops+1)
		}
	}
	if local {
		return
	}
	if text, ok := scope.pkg.consts[ident.Name]; ok {
		tr.judge(ident.Pos(), text)
	} else if held, ok := scope.pkg.vars[ident.Name]; ok {
		tr.expr(held.value, &traceScope{pkg: scope.pkg, file: held.file}, hops+1)
	}
}

// callers follows a parameter of an unbound function to the argument each call
// passes it: in-package for an unexported function, anywhere in the tree for an
// exported one, since a seam method's sentence is written by the module that
// calls it. A method is matched by name and arity alone, the closest a census
// without types gets to "this method".
func (tr *proseTrace) callers(scope *traceScope, at, hops int) {
	fn := scope.fn
	pkgs := []*summaryPackage{scope.pkg}
	if fn.Name.IsExported() {
		pkgs = sortedPackages(tr.tree.pkgs)
	}
	for _, pkg := range pkgs {
		for _, site := range pkg.calls {
			if calleeName(site.call) == fn.Name.Name && len(site.call.Args) == fn.Type.Params.NumFields() &&
				reachesDeclaration(site, fn, scope.pkg.dir, pkg == scope.pkg) {
				tr.expr(site.call.Args[at], &traceScope{pkg: pkg, file: site.file, fn: site.fn}, hops)
			}
		}
	}
}

// reachesDeclaration reports whether a call of the right name can reach fn: a
// method through any selector that is not a package qualifier, a function by
// its bare name at home or through an import of its package elsewhere.
func reachesDeclaration(site callIn, fn *ast.FuncDecl, dir string, home bool) bool {
	sel, isSelector := site.call.Fun.(*ast.SelectorExpr)
	if fn.Recv != nil {
		return isSelector && !isQualified(site.file, site.call.Fun)
	}
	if home {
		return !isSelector
	}
	imported, ok := importedDir(site.file, selectorX(sel))
	return ok && imported == dir
}

func selectorX(sel *ast.SelectorExpr) ast.Expr {
	if sel == nil {
		return nil
	}
	return sel.X
}

// selector passes a field off a local, a parameter or a copy table, and follows
// a constant of another in-module package or a field of a package-level
// literal, which are sentences written somewhere else in product code.
func (tr *proseTrace) selector(sel *ast.SelectorExpr, scope *traceScope, hops int) {
	if dir, ok := importedDir(scope.file, sel.X); ok {
		if pkg := tr.tree.pkgs[dir]; pkg != nil {
			if text, known := pkg.consts[sel.Sel.Name]; known {
				tr.judge(sel.Pos(), text)
			}
		}
		return
	}
	holder, ok := sel.X.(*ast.Ident)
	if !ok || tr.declared(scope, holder) {
		return
	}
	held, ok := scope.pkg.vars[holder.Name]
	if !ok {
		return
	}
	if lit, isLit := held.value.(*ast.CompositeLit); isLit {
		for _, elt := range lit.Elts {
			if kv, isKV := elt.(*ast.KeyValueExpr); isKV {
				if key, isIdent := kv.Key.(*ast.Ident); isIdent && key.Name == sel.Sel.Name {
					tr.expr(kv.Value, &traceScope{pkg: scope.pkg, file: held.file}, hops+1)
				}
			}
		}
	}
}

// index passes a table keyed by language — that is what a copy table is — and
// reads every value of any other package-level table, since the entry picked
// is a sentence the code wrote in one language.
func (tr *proseTrace) index(ix *ast.IndexExpr, scope *traceScope, hops int) {
	holder, ok := ix.X.(*ast.Ident)
	if !ok || tr.declared(scope, holder) {
		return
	}
	held, ok := scope.pkg.vars[holder.Name]
	if !ok {
		return
	}
	lit, ok := held.value.(*ast.CompositeLit)
	if !ok {
		return
	}
	if table, isMap := lit.Type.(*ast.MapType); isMap && strings.HasSuffix(typeKey(table.Key, held.file, scope.pkg.dir), ".Lang") {
		return
	}
	for _, elt := range lit.Elts {
		if kv, isKV := elt.(*ast.KeyValueExpr); isKV {
			elt = kv.Value
		}
		tr.expr(elt, &traceScope{pkg: scope.pkg, file: held.file}, hops+1)
	}
}

func summaryParamIndex(fn *ast.FuncDecl, name string) int {
	if fn == nil {
		return -1
	}
	at := 0
	for _, field := range fn.Type.Params.List {
		for _, ident := range field.Names {
			if ident.Name == name {
				return at
			}
			at++
		}
	}
	return -1
}

// assignment is one value a local is given: an expression, or the index-th
// result of a call that assigns several names at once.
type assignment struct {
	value ast.Expr
	call  *ast.CallExpr
	index int
}

func localAssignments(fn *ast.FuncDecl, name string) []assignment {
	if fn == nil || fn.Body == nil {
		return nil
	}
	var out []assignment
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			out = append(out, assignedTo(n.Lhs, n.Rhs, name)...)
		case *ast.ValueSpec:
			names := make([]ast.Expr, len(n.Names))
			for i, ident := range n.Names {
				names[i] = ident
			}
			out = append(out, assignedTo(names, n.Values, name)...)
		}
		return true
	})
	return out
}

func assignedTo(lhs, rhs []ast.Expr, name string) []assignment {
	var out []assignment
	for i, target := range lhs {
		ident, ok := target.(*ast.Ident)
		if !ok || ident.Name != name {
			continue
		}
		switch {
		case len(rhs) == len(lhs):
			out = append(out, assignment{value: rhs[i]})
		case len(rhs) == 1:
			if call, isCall := rhs[0].(*ast.CallExpr); isCall {
				out = append(out, assignment{call: call, index: i})
			} else if i == 0 {
				// `phrase, ok := table[key]`: the comma-ok forms carry the value first.
				out = append(out, assignment{value: rhs[0]})
			}
		}
	}
	return out
}

var proseFormatVerb = regexp.MustCompile(`%[-+# 0]*(\[\d+\])?(\d+|\*)?(\.(\d+|\*)?)?[a-zA-Z%]`)

// isEnglishProse reports whether a literal, with its format verbs removed, is
// words a reader reads: two of them, or one that is capitalised or set in a
// sentence ("Archive %s", "%d recipients"). A token joined by `_`, `:`, `=` or
// `/` is an identifier ("former_customer", "under="), so is a literal that is
// one lowercase word and nothing else ("offer"), and punctuation is layout.
func isEnglishProse(text string) bool {
	bare := proseFormatVerb.ReplaceAllString(text, "")
	var words []string
	for _, token := range strings.Fields(bare) {
		if strings.ContainsAny(token, "_:=/") || !strings.ContainsFunc(token, unicode.IsLetter) {
			continue
		}
		words = append(words, token)
	}
	switch len(words) {
	case 0:
		return false
	case 1:
		first, _ := utf8.DecodeRuneInString(strings.TrimLeftFunc(words[0], func(r rune) bool { return !unicode.IsLetter(r) }))
		return unicode.IsUpper(first) || bare != words[0]
	}
	return true
}
