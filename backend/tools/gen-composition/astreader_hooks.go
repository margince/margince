// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"go/ast"
	"sort"
)

// readWriteHooks reads WriteHooks into "<entity>.<action>" entries, so the
// manifest names every write a unit runs inside.
func (r *unitReader) readWriteHooks(expr ast.Expr, file *ast.File) ([]string, error) {
	var out []string
	err := r.eachHookLiteral(expr, file, "WriteHook", []string{"Entity", "Actions"}, func(fields map[string]ast.Expr) error {
		entity, err := r.stringLit(fields["Entity"], "WriteHook.Entity")
		if err != nil {
			return err
		}
		actions, err := r.stringList(fields["Actions"], "WriteHook.Actions")
		if err != nil {
			return err
		}
		for _, action := range actions {
			out = append(out, entity+"."+action)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

// readOperationHooks reads OperationHooks into the operationIds they run before.
func (r *unitReader) readOperationHooks(expr ast.Expr, file *ast.File) ([]string, error) {
	var out []string
	err := r.eachHookLiteral(expr, file, "OperationHook", []string{"Operations"}, func(fields map[string]ast.Expr) error {
		ops, err := r.stringList(fields["Operations"], "OperationHook.Operations")
		out = append(out, ops...)
		return err
	})
	sort.Strings(out)
	return out, err
}

// eachHookLiteral hands each keyed extension.<typeName> literal's fields to
// fn, once every required one is present. Handle is the behavior half and is
// not read, as a Tool's is not.
func (r *unitReader) eachHookLiteral(expr ast.Expr, file *ast.File, typeName string, required []string, fn func(map[string]ast.Expr) error) error {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return r.errAt(expr, "%ss must be a slice literal", typeName)
	}
	ext := importAlias(file, extensionPkgPath)
	for _, elt := range lit.Elts {
		hook, ok := elt.(*ast.CompositeLit)
		if !ok || (hook.Type != nil && !isSelector(hook.Type, ext, typeName)) {
			return r.errAt(elt, "a %ss entry must be an extension.%s literal", typeName, typeName)
		}
		fields := map[string]ast.Expr{}
		for _, e := range hook.Elts {
			kv, ok := e.(*ast.KeyValueExpr)
			if !ok {
				return r.errAt(e, "%s fields must be keyed by name", typeName)
			}
			key, named := kv.Key.(*ast.Ident)
			if !named {
				return r.errAt(e, "%s fields must be keyed by name", typeName)
			}
			fields[key.Name] = kv.Value
		}
		for _, name := range required {
			if fields[name] == nil {
				return r.errAt(hook, "%s needs %s", typeName, name)
			}
		}
		if err := fn(fields); err != nil {
			return err
		}
	}
	return nil
}

// stringList reads a non-empty slice literal of string literals.
func (r *unitReader) stringList(expr ast.Expr, field string) ([]string, error) {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok || len(lit.Elts) == 0 {
		return nil, r.errAt(expr, "%s must be a non-empty slice literal of strings", field)
	}
	out := make([]string, 0, len(lit.Elts))
	for _, elt := range lit.Elts {
		s, err := r.stringLit(elt, field)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}
