// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"go/ast"
	"slices"
	"strings"
)

type problemErrorMethod struct {
	fn   *ast.FuncDecl
	unit *goUnit
}

func (p *pairCensus) errorMethod(receiver string, unit *goUnit, name string, depth int) (problemErrorMethod, bool) {
	if depth > 10 {
		return problemErrorMethod{}, false
	}
	for _, owner := range p.codes.units {
		if owner.dir != unit.dir {
			continue
		}
		for _, decl := range owner.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != name || fn.Body == nil {
				continue
			}
			if typ, ok := methodReceiverType(fn); ok && typ == receiver {
				return problemErrorMethod{fn, owner}, true
			}
		}
	}
	shape := p.codes.structs[unit.dir][receiver]
	if shape == nil {
		return problemErrorMethod{}, false
	}
	for _, field := range shape.Fields.List {
		if len(field.Names) > 0 {
			continue
		}
		typ := field.Type
		if star, ok := typ.(*ast.StarExpr); ok {
			typ = star.X
		}
		full, ok := p.codes.literalType(unit, typ)
		if !ok {
			continue
		}
		dir, named, _ := strings.Cut(full, ".")
		nested := p.codes.structs[dir][named]
		if nested == nil {
			continue
		}
		if method, known := p.errorMethod(named, p.codes.structAt[nested], name, depth+1); known {
			return method, true
		}
	}
	return problemErrorMethod{}, false
}

func (p *pairCensus) finiteMessageStatuses(message *ast.FuncDecl, unit *goUnit) ([]int, bool) {
	receiver, ok := methodReceiverType(message)
	if !ok {
		return nil, false
	}
	method, wrapped := p.errorMethod(receiver, unit, "Unwrap", 0)
	if !wrapped {
		if inherited, known := p.errorMethod(receiver, unit, "Is", 0); known {
			typ, _ := methodReceiverType(inherited.fn)
			if statuses := p.isMethodStatuses(typ, inherited.unit); len(statuses) > 0 {
				return statuses, true
			}
		}
		return []int{422}, true
	}
	statuses, known := p.unwrapReturns(method, 0)
	if direct := p.isMethodStatuses(receiver, unit); len(direct) > 0 {
		return direct, true
	}
	return statuses, known
}

func (p *pairCensus) unwrapReturns(method problemErrorMethod, depth int) ([]int, bool) {
	var out []int
	known := true
	found := false
	ast.Inspect(method.fn.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		returned, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		if len(returned.Results) != 1 {
			known = false
			return false
		}
		values, ok := p.unwrapStatuses(returned.Results[0], method.unit, method.fn, depth+1)
		out = append(out, values...)
		known = known && ok
		found = true
		return false
	})
	slices.Sort(out)
	return slices.Compact(out), known && found
}

func (p *pairCensus) unwrapStatuses(expr ast.Expr, unit *goUnit, in *ast.FuncDecl, depth int) ([]int, bool) {
	if depth > 14 {
		return nil, false
	}
	if name, ok := expr.(*ast.Ident); ok && name.Name == "nil" {
		return []int{422}, true
	}
	if wanted, ok := p.codes.literalType(unit, expr); ok {
		var out []int
		for _, row := range p.codes.tableRows() {
			if target, known := p.codes.literalType(row.unit, row.sentinel); known && wanted == target {
				out = append(out, p.statuses(row.status, row.unit, nil, 0)...)
			}
		}
		if len(out) > 0 {
			return out, true
		}
	}
	switch value := expr.(type) {
	case *ast.ParenExpr:
		return p.unwrapStatuses(value.X, unit, in, depth+1)
	case *ast.UnaryExpr:
		return p.unwrapStatuses(value.X, unit, in, depth+1)
	case *ast.Ident:
		if bound, ok := p.codes.fieldBindings[value.Obj]; ok {
			return p.unwrapStatuses(bound.expr, bound.unit, bound.in, depth+1)
		}
		if assigned, known := assignedValue(value); known {
			return p.unwrapStatuses(assigned, unit, in, depth+1)
		}
		if at, parameter := codeParamIndex(in, value); parameter {
			var out []int
			known := true
			sites := p.codes.calls[codeFuncKey{dir: unit.dir, name: in.Name.Name, method: in.Recv != nil}]
			for _, site := range sites {
				if at >= len(site.call.Args) {
					known = false
					continue
				}
				got, ok := p.unwrapStatuses(site.call.Args[at], site.unit, site.in, depth+1)
				out = append(out, got...)
				known = known && ok
			}
			return out, known && len(sites) > 0
		}
	case *ast.SelectorExpr:
		if base, ok := value.X.(*ast.Ident); ok && in.Recv != nil {
			receiver, method := methodReceiverType(in)
			if method && len(in.Recv.List[0].Names) > 0 && in.Recv.List[0].Names[0].Obj == base.Obj {
				shape := p.codes.structs[unit.dir][receiver]
				var out []int
				known := true
				found := false
				for _, literal := range p.codes.composite[unit.dir+"."+receiver] {
					field, ok := fieldValue(literal.lit, shape, value.Sel.Name)
					if !ok {
						continue
					}
					got, ok := p.unwrapStatuses(field, literal.unit, literal.in, depth+1)
					out = append(out, got...)
					known = known && ok
					found = true
				}
				return out, known && found
			}
		}
	case *ast.CompositeLit:
		if array, ok := value.Type.(*ast.ArrayType); ok && array != nil {
			var out []int
			known := true
			for _, element := range value.Elts {
				got, ok := p.unwrapStatuses(element, unit, in, depth+1)
				out = append(out, got...)
				known = known && ok
			}
			return out, known
		}
	case *ast.CallExpr:
		if selected, ok := value.Fun.(*ast.SelectorExpr); ok && (selected.Sel.Name == "Errorf" || selected.Sel.Name == "Join") {
			var out []int
			known := true
			args := value.Args
			if selected.Sel.Name == "Errorf" && len(args) > 0 {
				args = args[1:]
			}
			for _, arg := range args {
				got, ok := p.unwrapStatuses(arg, unit, in, depth+1)
				out = append(out, got...)
				known = known && ok
			}
			return out, known && len(out) > 0
		}
		key, ok := p.codes.callee(unit, value.Fun)
		if !ok {
			return nil, false
		}
		fn := p.codes.funcs[key]
		if fn == nil || fn.Body == nil {
			return nil, false
		}
		restore := p.bind(fn, value, unit, in)
		defer restore()
		return p.unwrapReturns(problemErrorMethod{fn, p.codes.owner[fn]}, depth+1)
	}
	return nil, false
}
