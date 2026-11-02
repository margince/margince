// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

//gate:kind census H2

package gates

import (
	"go/ast"
	"regexp"
	"strconv"
)

var threadFormatVerb = regexp.MustCompile(`%(?:\[([0-9]+)\])?[-+#0 .0-9]*[a-zA-Z%]`)

func (p threadSQLPackage) formatValues(call *ast.CallExpr, bindings map[string][]ast.Expr, reached map[string]bool, context *ast.FuncDecl, depth int) []threadSQLValue {
	if len(call.Args) == 0 {
		return []threadSQLValue{{unresolved: true}}
	}
	var out []threadSQLValue
	arguments := map[int][]threadSQLValue{}
	for _, format := range p.values(call.Args[0], bindings, reached, context, depth+1) {
		values := []threadSQLValue{{unresolved: format.unresolved}}
		position, next := 0, 1
		for _, span := range threadFormatVerb.FindAllStringSubmatchIndex(format.text, -1) {
			values = joinThreadSQL(values, []threadSQLValue{{text: format.text[position:span[0]]}})
			verb := format.text[span[0]:span[1]]
			if verb == "%%" {
				values = joinThreadSQL(values, []threadSQLValue{{text: "%"}})
			} else {
				index := next
				if span[2] >= 0 {
					parsed, err := strconv.Atoi(format.text[span[2]:span[3]])
					if err != nil {
						return []threadSQLValue{{unresolved: true}}
					}
					index = parsed
				}
				if index >= len(call.Args) {
					return []threadSQLValue{{unresolved: true}}
				}
				argument, known := arguments[index]
				if !known {
					argument = p.values(call.Args[index], bindings, reached, context, depth+1)
					arguments[index] = argument
				}
				values = joinThreadSQL(values, argument)
				next = index + 1
			}
			position = span[1]
		}
		values = joinThreadSQL(values, []threadSQLValue{{text: format.text[position:]}})
		out = append(out, values...)
	}
	return out
}

func threadTransparentFormatter(fn *ast.FuncDecl) bool {
	if fn.Body == nil || len(fn.Body.List) != 1 {
		return false
	}
	returned, ok := fn.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 {
		return false
	}
	call, ok := returned.Results[0].(*ast.CallExpr)
	if !ok || !call.Ellipsis.IsValid() || len(call.Args) != 2 {
		return false
	}
	method, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || method.Sel.Name != "Sprintf" {
		return false
	}
	params := threadParameterNames(fn)
	if len(params) != 2 {
		return false
	}
	first, a := call.Args[0].(*ast.Ident)
	second, b := call.Args[1].(*ast.Ident)
	return a && b && first.Name == params[0] && second.Name == params[1]
}
