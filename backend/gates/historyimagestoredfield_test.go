// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"go/ast"
	"go/types"
	"regexp"
	"strconv"
	"strings"
)

var historySelectColumns = regexp.MustCompile(`(?is)SELECT\s+([a-z_.,\s]+)\s+FROM\s+([a-z_]+)\b`)
var historyInsertColumns = regexp.MustCompile(`(?is)INSERT\s+INTO\s+([a-z_]+)\s*\(([^)]+)\)\s*VALUES\s*\(([^)]+)\)`)

// A private field read back from a ledger has the vocabulary its insert stored.
// Require the same typed field at both ends; an arbitrary scanned string stays unreadable.
func (r historyImageReader) storedFieldStrings(in historyImageArg, selected *ast.SelectorExpr, depth int) ([]string, bool) {
	if depth > 32 {
		return nil, false
	}
	field, ok := in.pkg.info.Uses[selected.Sel].(*types.Var)
	if !ok || !field.IsField() || field.Exported() {
		return nil, false
	}
	field = field.Origin()
	writes := r.index.fields().writes[field]
	var values []string
	table, column := "", ""
	scanned := false
	for _, write := range writes {
		if write.value != nil {
			got, ok := r.strings(historyImageArg{pkg: write.pkg, decl: write.decl}, write.value, depth+1)
			if !ok {
				return nil, false
			}
			values = append(values, got...)
			continue
		}
		if write.decl == nil {
			return nil, false
		}
		t, c, ok := historyScannedField(write.pkg, write.decl, field)
		if !ok || (scanned && (t != table || c != column)) {
			return nil, false
		}
		table, column, scanned = t, c, true
	}
	if !scanned || len(values) == 0 {
		return nil, false
	}
	inserted := false
	known := true
	for at := range r.index.pkgs {
		pkg := &r.index.pkgs[at]
		for _, file := range pkg.files {
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok || len(call.Args) < 2 {
					return true
				}
				sql, ok := auditConstString(pkg.info, call.Args[1])
				if !ok {
					return true
				}
				if regexp.MustCompile(`(?is)\bUPDATE\s+` + regexp.QuoteMeta(table) + `\s+SET\s+` + regexp.QuoteMeta(column) + `\s*=`).MatchString(sql) {
					known = false
				}
				match := historyInsertColumns.FindStringSubmatch(sql)
				if match == nil || match[1] != table {
					return true
				}
				columns, arguments := strings.Split(match[2], ","), strings.Split(match[3], ",")
				for i, name := range columns {
					if strings.TrimSpace(name) != column {
						continue
					}
					if i >= len(arguments) {
						known = false
						continue
					}
					position, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(arguments[i]), "$"))
					if err != nil || position+1 >= len(call.Args) {
						known = false
						continue
					}
					member, ok := call.Args[position+1].(*ast.SelectorExpr)
					if !ok || auditSelectedField(pkg.info, member) != field {
						known = false
						continue
					}
					inserted = true
				}
				return true
			})
		}
	}
	return values, known && inserted
}

func historyScannedField(pkg *typedPackage, fn *ast.FuncDecl, field *types.Var) (string, string, bool) {
	position := -1
	var queries []string
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if len(call.Args) > 1 {
			if sql, ok := auditConstString(pkg.info, call.Args[1]); ok {
				queries = append(queries, sql)
			}
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Scan" {
			return true
		}
		for at, arg := range call.Args {
			address, ok := arg.(*ast.UnaryExpr)
			if ok && auditSelectedField(pkg.info, address.X) == field {
				position = at
			}
		}
		return true
	})
	if position < 0 {
		return "", "", false
	}
	for _, sql := range queries {
		match := historySelectColumns.FindStringSubmatch(sql)
		if match == nil {
			continue
		}
		columns := strings.Split(match[1], ",")
		if position < len(columns) {
			return match[2], strings.TrimSpace(columns[position]), true
		}
	}
	return "", "", false
}

// Private constructor fields can carry a local closure's parameter as their key.
func (r historyImageReader) privateFieldStrings(in historyImageArg, selected *ast.SelectorExpr, depth int) ([]string, bool) {
	if depth > 32 {
		return nil, false
	}
	field := auditSelectedField(in.pkg.info, selected)
	if field == nil || field.Exported() {
		return nil, false
	}
	if r.stringFieldBusy == nil {
		r.stringFieldBusy = map[*types.Var]bool{}
	}
	if r.stringFieldBusy[field] {
		return nil, false
	}
	r.stringFieldBusy[field] = true
	defer delete(r.stringFieldBusy, field)
	writes := r.index.fields().writes[field]
	var values []string
	for _, write := range writes {
		if write.value == nil {
			return nil, false
		}
		got, ok := r.strings(historyImageArg{pkg: write.pkg, decl: write.decl}, write.value, depth+1)
		if !ok {
			return nil, false
		}
		values = append(values, got...)
	}
	return values, len(writes) > 0
}
