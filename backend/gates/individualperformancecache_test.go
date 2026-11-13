// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package gates

// A cache the individual-performance path walk can trust. Some map field
// may store only values read through a check. It then hands back only
// checked values, so its comma-ok read checks on the branch where it hit.

import (
	"go/ast"
	"go/types"
)

// cacheWrite is one store into a map field, in the function it is written in.
type cacheWrite struct {
	decl  *types.Func
	write *ast.AssignStmt
}

// fieldCaches is what the pre-pass found of each map field: its writes. It
// also marks a field some other use could fill, as an alias or a filled literal.
type fieldCaches struct {
	writes   map[*types.Var][]cacheWrite
	escapes  map[*types.Var]bool
	held     map[*types.Var]bool
	watching map[*ast.AssignStmt]*writeWatch
}

// writeWatch is what a walk saw at one write: whether it reached it, and
// whether every time it did the path was covered.
type writeWatch struct {
	reached, covered bool
}

func newFieldCaches() fieldCaches {
	return fieldCaches{
		writes: map[*types.Var][]cacheWrite{}, escapes: map[*types.Var]bool{},
		held: map[*types.Var]bool{}, watching: map[*ast.AssignStmt]*writeWatch{},
	}
}

// mapField is the map-typed struct field an expression selects, or nil.
func mapField(expr ast.Expr, info *types.Info) *types.Var {
	sel, ok := ast.Unparen(expr).(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	selection, ok := info.Selections[sel]
	if !ok {
		return nil
	}
	field, ok := selection.Obj().(*types.Var)
	if !ok || !field.IsField() {
		return nil
	}
	if _, isMap := field.Type().Underlying().(*types.Map); !isMap {
		return nil
	}
	return field.Origin()
}

// censusCaches records every write into a map field in file, and each field
// some other use could fill unseen. A write inside a func literal escapes
// too: the walk of its function cannot say when the literal runs.
func (g *switchGraph) censusCaches(file *ast.File, info *types.Info) {
	indexed := map[*ast.SelectorExpr]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.IndexExpr:
			if sel, ok := ast.Unparen(node.X).(*ast.SelectorExpr); ok {
				indexed[sel] = true
			}
		case *ast.AssignStmt:
			g.censusEmptied(node, info, indexed)
		}
		return true
	})
	ast.Inspect(file, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && !indexed[sel] {
			if field := mapField(sel, info); field != nil {
				g.caches.escapes[field] = true
			}
		}
		if lit, ok := n.(*ast.CompositeLit); ok {
			g.censusFilledLiteral(lit, info)
		}
		return true
	})
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		obj, _ := info.Defs[fn.Name].(*types.Func)
		g.censusWrites(fn.Body, obj, info)
	}
}

// censusEmptied marks a field set to an empty map literal as indexed, since
// it fills the field with nothing.
func (g *switchGraph) censusEmptied(assign *ast.AssignStmt, info *types.Info, indexed map[*ast.SelectorExpr]bool) {
	for i, lhs := range assign.Lhs {
		sel, ok := ast.Unparen(lhs).(*ast.SelectorExpr)
		if !ok || i >= len(assign.Rhs) || mapField(sel, info) == nil {
			continue
		}
		if lit, ok := ast.Unparen(assign.Rhs[i]).(*ast.CompositeLit); ok && len(lit.Elts) == 0 {
			indexed[sel] = true
		}
	}
}

// censusFilledLiteral marks a field a struct literal fills with anything but
// an empty map literal.
func (g *switchGraph) censusFilledLiteral(lit *ast.CompositeLit, info *types.Info) {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		field, ok := info.Uses[key].(*types.Var)
		if !ok || !field.IsField() {
			continue
		}
		if _, isMap := field.Type().Underlying().(*types.Map); !isMap {
			continue
		}
		if value, ok := ast.Unparen(kv.Value).(*ast.CompositeLit); !ok || len(value.Elts) > 0 {
			g.caches.escapes[field.Origin()] = true
		}
	}
}

func (g *switchGraph) censusWrites(body *ast.BlockStmt, decl *types.Func, info *types.Info) {
	inLiteral := 0
	var visit func(n ast.Node) bool
	visit = func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncLit:
			inLiteral++
			ast.Inspect(node.Body, visit)
			inLiteral--
			return false
		case *ast.AssignStmt:
			for _, lhs := range node.Lhs {
				index, ok := ast.Unparen(lhs).(*ast.IndexExpr)
				if !ok {
					continue
				}
				if field := mapField(index.X, info); field != nil && (inLiteral > 0 || decl == nil) {
					g.caches.escapes[field] = true
				} else if field != nil {
					g.caches.writes[field] = append(g.caches.writes[field], cacheWrite{decl, node})
				}
			}
		}
		return true
	}
	ast.Inspect(body, visit)
}

// cacheHeld reports a map field every write of which stores a covered value.
// Each write's function is walked from an unchecked entry, so a write covered
// there is covered from any caller. A field still being judged is not held,
// so a read inside its own writer cannot vouch for itself.
func (g *switchGraph) cacheHeld(field *types.Var) bool {
	if held, seen := g.caches.held[field]; seen {
		return held
	}
	g.caches.held[field] = false
	writes := g.caches.writes[field]
	if g.caches.escapes[field] || len(writes) == 0 {
		return false
	}
	walk := newSwitchWalk(g)
	for _, write := range writes {
		g.caches.watching[write.write] = &writeWatch{covered: true}
	}
	for _, write := range writes {
		walk.walk(run{fn: fnValue{decl: write.decl}}, pathState{}, 0)
	}
	held := true
	for _, write := range writes {
		watch := g.caches.watching[write.write]
		held = held && watch.reached && watch.covered
		delete(g.caches.watching, write.write)
	}
	g.caches.held[field] = held
	return held
}

// observeWrite records the state a walk reaches a watched write in.
func (g *switchGraph) observeWrite(assign *ast.AssignStmt, s pathState) {
	if watch, ok := g.caches.watching[assign]; ok {
		watch.reached, watch.covered = true, watch.covered && s.covered()
	}
}

// cacheHit is the ok variable of a comma-ok read of a held cache, as in
// value, ok := r.cache[key], or nil.
func (w *pathWalk) cacheHit(lhs, rhs []ast.Expr) types.Object {
	if len(lhs) != 2 || len(rhs) != 1 {
		return nil
	}
	index, ok := ast.Unparen(rhs[0]).(*ast.IndexExpr)
	if !ok {
		return nil
	}
	field := mapField(index.X, w.frame.info)
	if field == nil || !w.sw.g.cacheHeld(field) {
		return nil
	}
	return assignedObject(lhs[1], w.frame.info)
}
