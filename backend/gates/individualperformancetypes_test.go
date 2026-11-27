// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package gates

// What the individual-performance path walk knows of code outside the
// packages it reads. A call into one is named by its declaration alone. An
// error is proved non-nil by its type or its returns.

import (
	"go/ast"
	"go/types"
	"strings"
)

// outsideCallee names a call of a function no walked package declares, as
// path.Func or path.Type.Method, or "" for any other call. A method promoted
// through an embedded field is named for the type that declares it.
func (g *switchGraph) outsideCallee(call *ast.CallExpr, info *types.Info) string {
	fn := calledFunc(call, info)
	if fn == nil || fn.Pkg() == nil || g.walked[fn.Pkg().Path()] {
		return ""
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return fn.Pkg().Path() + "." + fn.Name()
	}
	recv := sig.Recv().Type()
	if ptr, ok := recv.(*types.Pointer); ok {
		recv = ptr.Elem()
	}
	named, ok := types.Unalias(recv).(*types.Named)
	if !ok {
		return ""
	}
	return fn.Pkg().Path() + "." + named.Obj().Name() + "." + fn.Name()
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
