// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// How a unit may SPELL a handler type, and how the readers decide a Handle
// field is nil at the declaration.
//
// Split from astreader.go because it answers one question the three handler
// kinds share: Tool, Job and Inbound each have a Handle field, each accepts the
// bare `nil`, a conversion through the published type, and a conversion through
// a package-local alias of it — and each used to carry its own copy of that
// recursion, differing only in a type name.

import (
	"go/ast"
	"go/token"
)

// handlerAliasTypeNames are the published extension handler function types a
// unit's Handle field can be typed as: extension.ToolHandler,
// extension.JobHandler, extension.InboundHandler. collectHandlerAliases scans
// the unit's package for a local alias of any of these.
var handlerAliasTypeNames = map[string]bool{
	"ToolHandler":    true,
	"JobHandler":     true,
	"InboundHandler": true,
}

// collectHandlerAliases finds every package-level type ALIAS
// (`type X = extension.Y`, never a defined type `type X extension.Y`) of one
// of handlerAliasTypeNames, keyed by the aliased type's own name.
//
// A unit that writes `type Handler = extension.InboundHandler` and then
// `Handle: Handler(nil)` performs the identical conversion the published-type
// spelling does — Handler and extension.InboundHandler are ONE type, an
// alias declares no new one. Resolving only extension.InboundHandler(nil) and
// not this would read `Handler(nil)` as an ordinary one-argument call to
// unit-authored code, i.e. as a REAL handler: generation would succeed on a
// declaration boot's own Validate refuses as nil.
//
// A defined type is excluded on purpose: `type Handler extension.Y` makes a
// DIFFERENT type from extension.Y, so assigning a Handler value to a field
// typed extension.Y would not even compile without a second, explicit
// conversion this reader already sees directly — there is no alias to resolve.
func collectHandlerAliases(pkgs map[string][]*ast.File, extensionPkg string) map[string]map[string]bool {
	aliases := map[string]map[string]bool{}
	for _, files := range pkgs {
		for _, f := range files {
			ext := importAlias(f, extensionPkg)
			if ext == "" {
				continue
			}
			for _, decl := range f.Decls {
				d, ok := decl.(*ast.GenDecl)
				if !ok || d.Tok != token.TYPE {
					continue
				}
				for _, spec := range d.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok || !ts.Assign.IsValid() {
						continue
					}
					sel, ok := unwrapType(ts.Type).(*ast.SelectorExpr)
					if !ok {
						continue
					}
					pkgIdent, ok := sel.X.(*ast.Ident)
					if !ok || pkgIdent.Name != ext || !handlerAliasTypeNames[sel.Sel.Name] {
						continue
					}
					if aliases[sel.Sel.Name] == nil {
						aliases[sel.Sel.Name] = map[string]bool{}
					}
					aliases[sel.Sel.Name][ts.Name.Name] = true
				}
			}
		}
	}
	// AN ALIAS OF AN ALIAS is the same type, so it must resolve the same way.
	// `type H = extension.InboundHandler` is caught above because its right
	// side names the published package; `type H2 = H` is not, because its
	// right side is a bare identifier. Both spell the identical type, and a
	// reader that saw only the first would publish `H2(nil)` as a real handler
	// and leave boot to refuse it — the failure landing at a shipped binary's
	// startup rather than at the `make composition` its author runs.
	//
	// To a fixed point rather than one extra pass, because the chain has no
	// declared length: H3 = H2 is as legal as H2 = H, and stopping at a depth
	// would put a silent horizon in a check whose whole job is to have none.
	for {
		grew := false
		for _, files := range pkgs {
			for _, f := range files {
				for _, decl := range f.Decls {
					d, ok := decl.(*ast.GenDecl)
					if !ok || d.Tok != token.TYPE {
						continue
					}
					for _, spec := range d.Specs {
						ts, ok := spec.(*ast.TypeSpec)
						if !ok || !ts.Assign.IsValid() {
							continue
						}
						rhs, ok := unwrapType(ts.Type).(*ast.Ident)
						if !ok {
							continue
						}
						for _, names := range aliases {
							if names[rhs.Name] && !names[ts.Name.Name] {
								names[ts.Name.Name] = true
								grew = true
							}
						}
					}
				}
			}
		}
		if !grew {
			return aliases
		}
	}
}

// isStaticallyNilHandler reports whether expr is nil at the declaration, for
// a Handle field published as extension type name (one of ToolHandler,
// JobHandler, InboundHandler). The one check every handler field shares,
// parameterized only by which published type its conversion must name —
// Tool's, Job's and Inbound's own nil checks used to be three copies of this
// differing solely in that string.
//
// Three spellings reach here as the same nil function value: the bare `nil`;
// a conversion through the published type itself
// (extension.ToolHandler(nil)); and a conversion through a package-local
// alias of that type declared in the unit's own package (`type Handler =
// extension.ToolHandler`, `Handler(nil)`) — a unit author's shorthand for the
// identical conversion.
//
// The CallExpr arm checks the callee, not just the argument count, and that
// check is load-bearing, not decorative: a syntactic conversion and an
// ordinary one-argument call are indistinguishable by shape alone (`T(x)` and
// `f(x)` parse identically), so accepting any one-argument call whose sole
// argument is nil — without checking what is being called — would read
// `mustDial(nil)` as inert too, exempting a call that already ran, at
// declaration time, from every gate the caller has for a real handler.
// Anything else — a function name, a literal, any other call — is refused
// outright by the callers of this function; it never falls back to treating
// an unrecognized shape as inert.
func (r *unitReader) isStaticallyNilHandler(expr ast.Expr, ext, typeName string) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name == "nil"
	case *ast.CallExpr:
		if len(e.Args) != 1 {
			return false
		}
		if isSelector(e.Fun, ext, typeName) {
			return r.isStaticallyNilHandler(e.Args[0], ext, typeName)
		}
		if ident, ok := e.Fun.(*ast.Ident); ok && r.handlerAliases[typeName][ident.Name] {
			return r.isStaticallyNilHandler(e.Args[0], ext, typeName)
		}
		return false
	case *ast.ParenExpr:
		return r.isStaticallyNilHandler(e.X, ext, typeName)
	}
	return false
}

// unwrapType strips redundant parentheses from a type expression. `type H2 =
// (H)` names the same type as `type H2 = H`, and a reader that saw only the
// second would publish a nil spelled through the first.
func unwrapType(expr ast.Expr) ast.Expr {
	for {
		paren, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}
		expr = paren.X
	}
}
