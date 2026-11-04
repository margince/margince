// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
)

// contractRoutersVar is the agents package's list of routers that have only
// contract actions.
const contractRoutersVar = "contractRouters"

// registeredRouters reads the router names from the agents package's source.
// The generator runs before that package need compile, so it cannot ask a
// registry. A router is a member of contractRouters, or a type with a routeTargets
// method, named by the Name its Spec gives.
func registeredRouters(dir string) (map[string]bool, error) {
	fset := token.NewFileSet()
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, fmt.Errorf("reading the routers in %s: %w", dir, err)
	}
	routers := map[string]bool{}
	targeting, specs, constants := map[string]bool{}, map[string]*ast.FuncDecl{}, map[string]string{}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("reading the routers in %s: %w", path, err)
		}
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				switch receiver := receiverName(d); d.Name.Name {
				case "routeTargets":
					targeting[receiver] = true
				case "Spec":
					specs[receiver] = d
				}
			case *ast.GenDecl:
				collectConstants(d, constants)
				collectContractRouters(d, constants, routers)
			}
		}
	}
	for receiver := range targeting {
		if spec := specs[receiver]; spec != nil {
			collectNames(spec, constants, routers)
		}
	}
	if len(routers) == 0 {
		return nil, fmt.Errorf("%s declares no routers, so every route-backed action would be refused", dir)
	}
	return routers, nil
}

func receiverName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) != 1 {
		return ""
	}
	recv := fn.Recv.List[0].Type
	if star, isPointer := recv.(*ast.StarExpr); isPointer {
		recv = star.X
	}
	if name, isName := recv.(*ast.Ident); isName {
		return name.Name
	}
	return ""
}

// collectConstants records the string constants a Name may be spelled with.
func collectConstants(decl *ast.GenDecl, into map[string]string) {
	if decl.Tok != token.CONST {
		return
	}
	for _, spec := range decl.Specs {
		value, isValue := spec.(*ast.ValueSpec)
		if !isValue {
			continue
		}
		for i, name := range value.Names {
			if i < len(value.Values) {
				if text, isText := stringLiteral(value.Values[i]); isText {
					into[name.Name] = text
				}
			}
		}
	}
}

func stringLiteral(expr ast.Expr) (string, bool) {
	literal, isLiteral := expr.(*ast.BasicLit)
	if !isLiteral || literal.Kind != token.STRING {
		return "", false
	}
	text, err := strconv.Unquote(literal.Value)
	return text, err == nil
}

func collectContractRouters(decl *ast.GenDecl, constants map[string]string, into map[string]bool) {
	for _, spec := range decl.Specs {
		if value, isValue := spec.(*ast.ValueSpec); isValue && len(value.Names) == 1 && value.Names[0].Name == contractRoutersVar {
			collectNames(value, constants, into)
		}
	}
}

// collectNames adds every Name field under node, spelled as a string or as a
// constant.
func collectNames(node ast.Node, constants map[string]string, into map[string]bool) {
	ast.Inspect(node, func(n ast.Node) bool {
		field, isField := n.(*ast.KeyValueExpr)
		if !isField {
			return true
		}
		if key, isKey := field.Key.(*ast.Ident); !isKey || key.Name != "Name" {
			return true
		}
		if text, isText := stringLiteral(field.Value); isText {
			into[text] = true
		}
		if name, isConstant := field.Value.(*ast.Ident); isConstant {
			if text, known := constants[name.Name]; known {
				into[text] = true
			}
		}
		return true
	})
}
