// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// A member's principal is built in authz.MemberPrincipal or authz.HumanPrincipal.
// Outside authz no code may move an authz.RBAC's Permissions or TeamIDs into a
// principal, by literal or assignment, or set Permissions from a local variable.
// Types decide, so aliased imports and renames change nothing.

import (
	"go/ast"
	"go/types"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	authzPackagePath     = modulePath + "/internal/shared/ports/authz"
	principalPackagePath = modulePath + "/internal/shared/kernel/principal"
)

// minimumMemberPrincipalCalls is the count of calls to authz.MemberPrincipal
// and authz.HumanPrincipal outside the authz package. A walk that stops seeing
// them has stopped seeing the callers this gate protects.
const minimumMemberPrincipalCalls = 20

func isNamedType(t types.Type, pkgPath, name string) bool {
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == pkgPath && named.Obj().Name() == name
}

func isGrantField(name string) bool { return name == "Permissions" || name == "TeamIDs" }

type principalCensus struct {
	info *types.Info
	// literals counts principal.Principal literals seen, wherever they are.
	literals int
}

func (c *principalCensus) typeIs(expr ast.Expr, pkgPath, name string) bool {
	tv, ok := c.info.Types[expr]
	return ok && isNamedType(tv.Type, pkgPath, name)
}

// readsResolvedGrants reports whether expr takes Permissions or TeamIDs out of
// an authz.RBAC anywhere inside it.
func (c *principalCensus) readsResolvedGrants(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && isGrantField(sel.Sel.Name) && c.typeIs(sel.X, authzPackagePath, "RBAC") {
			found = true
		}
		return !found
	})
	return found
}

// setsFromAnAuthority reports whether value, bound to a principal's grant
// field, is a resolved authority. Another principal's field or a session's own
// field is not.
func (c *principalCensus) setsFromAnAuthority(field string, value ast.Expr) bool {
	if c.readsResolvedGrants(value) {
		return true
	}
	if field != "Permissions" || !c.typeIs(value, principalPackagePath, "Permissions") {
		return false
	}
	switch value.(type) {
	case *ast.Ident:
		return true
	}
	return false
}

// handBuilt returns what in n builds a principal from an authority by hand.
func (c *principalCensus) handBuilt(n ast.Node) []ast.Node {
	var out []ast.Node
	switch node := n.(type) {
	case *ast.CompositeLit:
		if !c.typeIs(node, principalPackagePath, "Principal") {
			return nil
		}
		c.literals++
		for _, elt := range node.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			if key, ok := kv.Key.(*ast.Ident); ok && isGrantField(key.Name) && c.setsFromAnAuthority(key.Name, kv.Value) {
				out = append(out, kv)
			}
		}
	case *ast.AssignStmt:
		for i, lhs := range node.Lhs {
			sel, ok := lhs.(*ast.SelectorExpr)
			if !ok || !isGrantField(sel.Sel.Name) || !c.typeIs(sel.X, principalPackagePath, "Principal") {
				continue
			}
			sources := node.Rhs
			if len(node.Lhs) == len(node.Rhs) {
				sources = node.Rhs[i : i+1]
			}
			for _, src := range sources {
				if c.setsFromAnAuthority(sel.Sel.Name, src) && (len(out) == 0 || out[len(out)-1] != ast.Node(node)) {
					out = append(out, node)
				}
			}
		}
	}
	return out
}

// isAuthzBuilderCall matches a call to authz.MemberPrincipal or
// authz.HumanPrincipal, resolved by the callee's package and name.
func (c *principalCensus) isAuthzBuilderCall(n ast.Node) bool {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return false
	}
	var ident *ast.Ident
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		ident = fun.Sel
	case *ast.Ident:
		ident = fun
	default:
		return false
	}
	fn, ok := c.info.Uses[ident].(*types.Func)
	return ok && fn.Pkg() != nil && fn.Pkg().Path() == authzPackagePath &&
		(fn.Name() == "MemberPrincipal" || fn.Name() == "HumanPrincipal")
}

func TestNoCodeOutsideAuthzBuildsAMemberPrincipalByHand(t *testing.T) {
	t.Parallel()

	literals, builderCalls := 0, 0
	for _, pkg := range typedModule(t) {
		inAuthz := pkg.path == authzPackagePath
		census := &principalCensus{info: pkg.info}
		for _, file := range pkg.files {
			ast.Inspect(file, func(n ast.Node) bool {
				if !inAuthz && census.isAuthzBuilderCall(n) {
					builderCalls++
				}
				for _, site := range census.handBuilt(n) {
					if inAuthz {
						continue
					}
					t.Errorf("%s (%s) builds a principal from a resolved authority by hand.\n"+
						"Use authz.MemberPrincipal, or authz.HumanPrincipal when the read ran inside "+
						"your own transaction, then set what is yours (ID, Type, OnBehalfOf) on the result. "+
						"A hand-built principal that leaves out SeatType is judged as a full seat.",
						gatekit.SourceFileSet().Position(site.Pos()), pkg.path)
				}
				return true
			})
		}
		literals += census.literals
	}
	if literals == 0 {
		t.Fatal("this census saw no principal.Principal literal, so the type match has stopped working")
	}
	if builderCalls < minimumMemberPrincipalCalls {
		t.Fatalf("this census saw %d call(s) to authz.MemberPrincipal or authz.HumanPrincipal outside authz and expects at "+
			"least %d; the walk has stopped seeing the callers it protects", builderCalls, minimumMemberPrincipalCalls)
	}
}
