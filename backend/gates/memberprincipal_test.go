// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// A member's principal is built in one place: authz.MemberPrincipal, or
// authz.HumanPrincipal for a caller that resolved the authority inside its own
// transaction.
//
// A hand-built principal that leaves out SeatType is judged as a full seat.
//
// The census is every principal literal in non-test code that copies
// Permissions or TeamIDs from another value. Only the authz package may do
// that; there is no skip list, so a new site fails here. Rebinding fresh
// grants onto an existing principal by assignment is a different act and is
// not a construction.

import (
	"go/ast"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// isPrincipalLiteral matches a composite literal of principal.Principal, the
// type this package's callers build.
func isPrincipalLiteral(lit *ast.CompositeLit) bool {
	switch typ := lit.Type.(type) {
	case *ast.SelectorExpr:
		pkg, ok := typ.X.(*ast.Ident)
		return ok && pkg.Name == "principal" && typ.Sel.Name == "Principal"
	case *ast.Ident:
		return typ.Name == "Principal"
	}
	return false
}

var authorityReads = map[string]bool{
	"EffectiveAuthority":   true,
	"EffectiveAuthorityTx": true,
	"EffectiveRBAC":        true,
	"AdmittedAuthority":    true,
	"HeldAuthorityTx":      true,
}

// resolvedAuthorities names the variables in fn that hold a member's resolved
// RBAC: the first result of an authority read, or a parameter typed authz.RBAC.
// A session's own Identity or another principal also has a Permissions field;
// copying from those is not reading a member's authority.
func resolvedAuthorities(fn *ast.FuncDecl) map[string]bool {
	names := map[string]bool{}
	for _, field := range fn.Type.Params.List {
		if !isRBACType(field.Type) {
			continue
		}
		for _, name := range field.Names {
			names[name.Name] = true
		}
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Rhs) != 1 {
			return true
		}
		call, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && authorityReads[sel.Sel.Name] {
			if first, ok := assign.Lhs[0].(*ast.Ident); ok {
				names[first.Name] = true
			}
		}
		return true
	})
	return names
}

// isRBACType matches authz.RBAC, and the bare RBAC inside the authz package.
func isRBACType(expr ast.Expr) bool {
	switch typ := expr.(type) {
	case *ast.SelectorExpr:
		return typ.Sel.Name == "RBAC"
	case *ast.Ident:
		return typ.Name == "RBAC"
	}
	return false
}

// copiesGrantsInto reports whether the literal sets Permissions or TeamIDs
// from a resolved authority, which is how that authority becomes a principal.
func copiesGrantsInto(lit *ast.CompositeLit, resolved map[string]bool) bool {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || (key.Name != "Permissions" && key.Name != "TeamIDs") {
			continue
		}
		sel, ok := kv.Value.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != key.Name {
			continue
		}
		if from, ok := sel.X.(*ast.Ident); ok && resolved[from.Name] {
			return true
		}
	}
	return false
}

func TestNoCodeOutsideAuthzBuildsAMemberPrincipalByHand(t *testing.T) {
	t.Parallel()

	literals := 0
	for _, root := range []string{"internal", "cmd"} {
		err := gatekit.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			path = filepath.ToSlash(path)
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") || isIntegrationTagged(path) {
				return err
			}
			file, parseErr := gatekit.ParseFile(path, 0)
			if parseErr != nil {
				return parseErr
			}
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				literals += checkMemberPrincipalLiterals(t, path, fn)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	if literals < 1 {
		t.Fatalf("this census saw %d principal literal(s) copying a resolved authority; authz.HumanPrincipal "+
			"is one, so the walk has stopped recognising the construction", literals)
	}
}

// checkMemberPrincipalLiterals counts the hand-built member principals in fn
// and fails each one that sits outside the authz package.
func checkMemberPrincipalLiterals(t *testing.T, path string, fn *ast.FuncDecl) int {
	t.Helper()
	resolved := resolvedAuthorities(fn)
	count := 0
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok || !isPrincipalLiteral(lit) || !copiesGrantsInto(lit, resolved) {
			return true
		}
		count++
		if strings.HasPrefix(path, "internal/shared/ports/authz/") {
			return true
		}
		t.Errorf("%s:%d builds a principal from a resolved authority by hand.\n"+
			"Use authz.MemberPrincipal, or authz.HumanPrincipal when the read ran inside "+
			"your own transaction, then set what is yours (ID, Type, OnBehalfOf) on the result. "+
			"A hand-built principal that leaves out SeatType is judged as a full seat.",
			path, gatekit.SourceFileSet().Position(lit.Pos()).Line)
		return true
	})
	return count
}
