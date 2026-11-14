// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package gates

// What the individual-performance path walk knows of code outside the
// packages it type-checks. Each import outside them is an empty stand-in, so
// a call into one is named by its import path and declared type alone.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"regexp"
	"strings"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// switchImports answers a walked package with its real types, and any other
// with an empty package named the way code spells it.
type switchImports map[string]*types.Package

func (s switchImports) Import(importPath string) (*types.Package, error) {
	if pkg, ok := s[importPath]; ok {
		return pkg, nil
	}
	pkg := types.NewPackage(importPath, standInName(importPath))
	pkg.MarkComplete()
	return pkg, nil
}

// standInName is the name a package is declared with. A package of this
// module says so in its own source, since its name may differ from its path
// (crmcontracts for internal/contracts). Any other follows Go's convention:
// pgx for github.com/jackc/pgx/v5, yaml for gopkg.in/yaml.v3.
func standInName(importPath string) string {
	if dir, ours := strings.CutPrefix(importPath, modulePath+"/"); ours {
		if name := declaredPackageName(dir); name != "" {
			return name
		}
	}
	elems := strings.Split(importPath, "/")
	name := elems[len(elems)-1]
	if len(elems) > 1 && majorVersion.MatchString(name) {
		name = elems[len(elems)-2]
	}
	name, _, _ = strings.Cut(name, ".")
	return strings.ReplaceAll(strings.TrimPrefix(name, "go-"), "-", "")
}

// declaredPackageName reads the package clause of a Go file in dir, or ""
// when dir holds none.
func declaredPackageName(dir string) string {
	entries, err := gatekit.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if file, err := gatekit.ParseFile(dir+"/"+name, parser.PackageClauseOnly); err == nil {
			return file.Name.Name
		}
	}
	return ""
}

var majorVersion = regexp.MustCompile(`^v[0-9]+$`)

// standInErrors are the messages a stand-in import leaves behind. They name
// a value of a type it left invalid, or an import whose every use it leaves
// undeclared.
var standInErrors = regexp.MustCompile(`invalid type|unknown type|imported (as \w+ )?and not used|cannot infer|missing type in composite literal`)

// unexplainedTypeErrors lists the type errors of one package that no stand-in
// import explains.
func unexplainedTypeErrors(errs []types.Error, files []*ast.File, info *types.Info, walked switchImports) []string {
	selectors := map[token.Pos]*ast.SelectorExpr{}
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				selectors[sel.Pos()], selectors[sel.Sel.Pos()] = sel, sel
			}
			return true
		})
	}
	invalidVars := map[token.Pos]bool{}
	for ident, obj := range info.Defs {
		if v, ok := obj.(*types.Var); ok && !validType(v.Type()) {
			invalidVars[ident.Pos()] = true
		}
	}
	var out []string
	for _, err := range errs {
		if standInErrors.MatchString(err.Msg) || explainedSelector(selectors[err.Pos], info, walked) {
			continue
		}
		// A variable a stand-in left untyped is used only inside expressions
		// the checker skips, so it reads as unused.
		if strings.HasPrefix(err.Msg, "declared and not used") && invalidVars[err.Pos] {
			continue
		}
		out = append(out, fmt.Sprintf("%s: %s", err.Fset.Position(err.Pos), err.Msg))
	}
	return out
}

// explainedSelector reports a selector a stand-in leaves unresolved. That is
// a name of a stand-in package, or a field or method promoted through a field
// a stand-in left untyped.
func explainedSelector(sel *ast.SelectorExpr, info *types.Info, walked switchImports) bool {
	if sel == nil {
		return false
	}
	if ident, ok := ast.Unparen(sel.X).(*ast.Ident); ok {
		if pkg, ok := info.Uses[ident].(*types.PkgName); ok {
			_, real := walked[pkg.Imported().Path()]
			return !real
		}
	}
	return embedsInvalid(info.TypeOf(sel.X), map[types.Type]bool{})
}

// embedsInvalid reports a type a stand-in left without its methods. It is
// defined over a stand-in's type, or embeds one at any depth.
func embedsInvalid(t types.Type, seen map[types.Type]bool) bool {
	if t == nil || seen[t] {
		return false
	}
	seen[t] = true
	if ptr, ok := t.Underlying().(*types.Pointer); ok {
		t = ptr.Elem()
	}
	if param, ok := t.(*types.TypeParam); ok {
		return !validType(param.Constraint()) || embedsInvalid(param.Constraint(), seen)
	}
	switch under := t.Underlying().(type) {
	case *types.Basic:
		return under.Kind() == types.Invalid
	case *types.Struct:
		for field := range under.Fields() {
			if field.Embedded() && (!validType(field.Type()) || embedsInvalid(field.Type(), seen)) {
				return true
			}
		}
	case *types.Interface:
		for embedded := range under.EmbeddedTypes() {
			if !validType(embedded) || embedsInvalid(embedded, seen) {
				return true
			}
		}
	}
	return false
}

// validType reports a type no stand-in left invalid, in itself or in the
// element it is built of.
func validType(t types.Type) bool {
	switch e := t.(type) {
	case *types.Basic:
		return e.Kind() != types.Invalid
	case *types.Pointer:
		return validType(e.Elem())
	case *types.Slice:
		return validType(e.Elem())
	case *types.Array:
		return validType(e.Elem())
	case *types.Map:
		return validType(e.Key()) && validType(e.Elem())
	}
	return true
}

// spell records the declared type of each name whose type is a stand-in's,
// as path.Type, so a method called on it can be named.
func (g *switchGraph) spell(names []*ast.Ident, typeExpr ast.Expr, info *types.Info) {
	spelled := spelledType(typeExpr, info)
	if spelled == "" {
		return
	}
	for _, name := range names {
		if v, ok := info.Defs[name].(*types.Var); ok {
			g.spelled[v] = spelled
		}
	}
}

func spelledType(typeExpr ast.Expr, info *types.Info) string {
	if star, ok := typeExpr.(*ast.StarExpr); ok {
		typeExpr = star.X
	}
	sel, ok := typeExpr.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return ""
	}
	if pkg, ok := info.Uses[ident].(*types.PkgName); ok {
		return pkg.Imported().Path() + "." + sel.Sel.Name
	}
	return ""
}

// spellFromCall records the declared result type of a walked function for
// each variable defined from its call, as db := InstallationDB(pool).
func (g *switchGraph) spellFromCall(assign *ast.AssignStmt, info *types.Info) {
	call, ok := ast.Unparen(assign.Rhs[0]).(*ast.CallExpr)
	if assign.Tok != token.DEFINE || len(assign.Rhs) != 1 || !ok {
		return
	}
	decl := g.decls[calledFunc(call, info)]
	if decl == nil || decl.Type.Results == nil {
		return
	}
	var results []ast.Expr
	for _, field := range decl.Type.Results.List {
		for range max(1, len(field.Names)) {
			results = append(results, field.Type)
		}
	}
	if len(results) != len(assign.Lhs) {
		return
	}
	for i, lhs := range assign.Lhs {
		ident, isIdent := lhs.(*ast.Ident)
		spelled := spelledType(results[i], g.infos[calledFunc(call, info)])
		if v, ok := info.Defs[ident].(*types.Var); isIdent && ok && spelled != "" {
			g.spelled[v] = spelled
		}
	}
}

// outsideCallee names a call of a function no walked package declares, as
// path.Func or path.Type.Method, or "" for any other call.
func (g *switchGraph) outsideCallee(call *ast.CallExpr, info *types.Info) string {
	sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	if ident, ok := ast.Unparen(sel.X).(*ast.Ident); ok {
		if pkg, ok := info.Uses[ident].(*types.PkgName); ok {
			if pkg.Imported().Scope().Lookup(sel.Sel.Name) != nil {
				return ""
			}
			return pkg.Imported().Path() + "." + sel.Sel.Name
		}
	}
	if validType(typeOrInvalid(info.TypeOf(sel.X))) {
		return ""
	}
	var v *types.Var
	switch x := ast.Unparen(sel.X).(type) {
	case *ast.Ident:
		v, _ = info.Uses[x].(*types.Var)
	case *ast.SelectorExpr:
		if selection, ok := info.Selections[x]; ok {
			v, _ = selection.Obj().(*types.Var)
		}
	}
	if v == nil || g.spelled[v.Origin()] == "" {
		return ""
	}
	return g.spelled[v.Origin()] + "." + sel.Sel.Name
}

func typeOrInvalid(t types.Type) types.Type {
	if t == nil {
		return types.Typ[types.Invalid]
	}
	return t
}

// censusReassigned records each variable an assignment names. A package
// variable is then no sentinel, and a parameter no longer resolves from what
// its call site passed.
func (g *switchGraph) censusReassigned(assign *ast.AssignStmt, info *types.Info) {
	for _, lhs := range assign.Lhs {
		if v, ok := assignedObject(ast.Unparen(lhs), info).(*types.Var); ok {
			g.reassigned[v] = true
		}
	}
}

// isSentinel reports a package variable that is never nil: declared with an
// error no path can make nil and never assigned again.
func (g *switchGraph) isSentinel(obj types.Object) bool {
	v, ok := obj.(*types.Var)
	if !ok || !isPackageVar(v) || g.reassigned[v] {
		return false
	}
	if known, seen := g.sentinels[v]; seen {
		return known
	}
	g.sentinels[v] = false
	site, declared := g.inits[v]
	proved := declared && g.provedError(site.expr, site.info)
	g.sentinels[v] = proved
	return proved
}

// provedError reports an expression whose error the types and declarations
// prove non-nil, on every path.
func (g *switchGraph) provedError(expr ast.Expr, info *types.Info) bool {
	resolver := &pathWalk{sw: &switchWalk{g: g}, frame: &pathFrame{info: info}}
	return resolver.refuses(expr, pathState{}, false)
}

// declaredSentinel reports an Err name of a package the config declares
// holds only errors that are never nil.
func (g *switchGraph) declaredSentinel(sel *ast.SelectorExpr, info *types.Info) bool {
	ident, ok := ast.Unparen(sel.X).(*ast.Ident)
	if !ok {
		return false
	}
	pkg, ok := info.Uses[ident].(*types.PkgName)
	if !ok || !strings.HasPrefix(sel.Sel.Name, "Err") {
		return false
	}
	for _, declared := range g.config.sentinelPackages {
		if pkg.Imported().Path() == declared {
			return true
		}
	}
	return false
}

// alwaysRefuses reports a walked function whose every return hands back an
// error proved non-nil. A function whose last result is a concrete error type
// that holds no nil refuses too.
func (g *switchGraph) alwaysRefuses(fn *types.Func) bool {
	decl := g.decls[fn]
	if decl == nil || decl.Type.Results == nil {
		return false
	}
	if known, seen := g.refusing[fn]; seen {
		return known
	}
	g.refusing[fn] = false
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Results().Len() == 0 {
		return false
	}
	last := sig.Results().At(sig.Results().Len() - 1).Type()
	proved := !types.IsInterface(last) && types.AssignableTo(last, errorType) && cannotBeNil(last)
	if !proved && types.Identical(last, errorType) {
		proved = g.everyReturnRefuses(decl, g.infos[fn], sig.Results().Len())
	}
	g.refusing[fn] = proved
	return proved
}

func cannotBeNil(t types.Type) bool {
	switch t.Underlying().(type) {
	case *types.Pointer, *types.Interface, *types.Map, *types.Slice, *types.Signature, *types.Chan:
		return false
	}
	return true
}

func (g *switchGraph) everyReturnRefuses(decl *ast.FuncDecl, info *types.Info, results int) bool {
	proved, returns := true, 0
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			returns++
			proved = proved && len(node.Results) == results && g.provedError(node.Results[results-1], info)
		}
		return proved
	})
	return proved && returns > 0
}
