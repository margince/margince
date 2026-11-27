// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"go/ast"
	"go/types"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
)

type historyStoredCacheKey struct {
	index  *auditIndex
	field  *types.Var
	flavor int
}
type historyStoredCacheValue struct {
	values []string
	known  bool
}

var historyStoredReads sync.Map

var (
	historySelectColumns    = regexp.MustCompile(`(?is)SELECT\s+(.+?)\s+FROM\s+([a-z_]+)\b`)
	historyUpdateAssignment = regexp.MustCompile(`(?is)^\s*(?:"([a-z_][a-z_0-9]*)"|([a-z_][a-z_0-9]*))\s*=`)
	historyInsertColumns    = regexp.MustCompile(`(?is)INSERT\s+INTO\s+([a-z_]+)\s*\(([^)]+)\)\s*VALUES\s*\(([^)]+)\)`)
)

// A private field read back from a ledger has the vocabulary its insert stored.
// Require the same typed field at both ends; an arbitrary scanned string stays unreadable.
func (r historyImageReader) storedFieldStrings(in historyImageArg, selected *ast.SelectorExpr, depth int) ([]string, bool) {
	if r.storedFieldBusy == nil {
		r.storedFieldBusy = map[*types.Var]bool{}
	}
	return r.storedFieldValues(in, selected, depth, false, func(child historyImageArg, d int) ([]string, bool) { return r.strings(child, child.expr, d) })
}

func (r historyImageReader) storedFieldValues(in historyImageArg, selected *ast.SelectorExpr, depth int, array bool, read func(historyImageArg, int) ([]string, bool)) (cacheValues []string, cacheKnown bool) {
	if depth > 32 {
		return nil, false
	}
	field, ok := in.pkg.info.Uses[selected.Sel].(*types.Var)
	if !ok || !field.IsField() || (field.Exported() && !array) {
		return nil, false
	}
	field = field.Origin()
	flavor := 1
	if array {
		flavor = 2
	}
	cacheKey := historyStoredCacheKey{r.index, field, flavor}
	if cached, ok := historyStoredReads.Load(cacheKey); ok {
		value, valid := cached.(historyStoredCacheValue)
		if !valid {
			return nil, false
		}
		return slices.Clone(value.values), value.known
	}
	outer := len(r.storedFieldBusy) == 0
	if outer {
		defer func() {
			historyStoredReads.Store(cacheKey, historyStoredCacheValue{slices.Clone(cacheValues), cacheKnown})
		}()
	}
	if r.storedFieldBusy == nil {
		r.storedFieldBusy = map[*types.Var]bool{}
	}
	if r.storedFieldBusy[field] {
		return nil, true
	}
	r.storedFieldBusy[field] = true
	defer delete(r.storedFieldBusy, field)
	writes := r.index.fields().writes[field]
	var values []string
	table, column := "", ""
	scanned := false
	for _, write := range writes {
		if write.value != nil {
			got, ok := read(historyImageArg{expr: write.value, pkg: write.pkg, decl: write.decl}, depth+1)
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
	relation := `(?:"?[a-z_][a-z_0-9]*"?\.)?"?` + regexp.QuoteMeta(table) + `"?`
	writeTable := regexp.MustCompile(`(?is)\b(?:INSERT\s+INTO|UPDATE(?:\s+ONLY)?|MERGE\s+INTO|COPY)\s+` + relation + `(?:\s|\(|$)`)
	updateTable := regexp.MustCompile(`(?is)\bUPDATE\s+(?:ONLY\s+)?` + relation + `(?:\s+(?:AS\s+)?[a-z_][a-z_0-9]*)?\s+SET\s+(.+?)(?:\bWHERE\b|\bRETURNING\b|$)`)
	accounted := map[string]bool{}
	var fragments []string
	for at := range r.index.pkgs {
		pkg := &r.index.pkgs[at]
		for _, file := range pkg.files {
			ast.Inspect(file, func(node ast.Node) bool {
				if literal, ok := node.(*ast.BasicLit); ok {
					if text, ok := auditConstString(pkg.info, literal); ok && writeTable.MatchString(text) {
						fragments = append(fragments, text)
					}
				}
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				for argAt, arg := range call.Args {
					sql, ok := auditConstString(pkg.info, arg)
					if !ok {
						continue
					}
					if selected, ok := call.Fun.(*ast.SelectorExpr); ok && selected.Sel.Name == "InsertFragments" && sql == table {
						known = false
					}
					if !writeTable.MatchString(sql) {
						continue
					}
					accounted[sql] = true
					if updateTable.MatchString(sql) {
						match := updateTable.FindStringSubmatch(sql)
						if !historyUpdateLeavesColumn(match[1], column) {
							known = false
						}
						continue
					}
					match := historyInsertColumns.FindStringSubmatch(sql)
					if match == nil || match[1] != table {
						known = false
						continue
					}
					columns, arguments := strings.Split(match[2], ","), strings.Split(match[3], ",")
					found := false
					for i, name := range columns {
						if strings.TrimSpace(name) != column {
							continue
						}
						found = true
						if i >= len(arguments) {
							known = false
							continue
						}
						position, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(arguments[i]), "$"))
						if err != nil || position < 1 || argAt+position >= len(call.Args) {
							known = false
							continue
						}
						member, ok := call.Args[argAt+position].(*ast.SelectorExpr)
						if !ok || auditSelectedField(pkg.info, member) != field {
							known = false
							continue
						}
						inserted = true
					}
					if !found {
						known = false
					}
				}
				return true
			})
		}
	}
	for _, fragment := range fragments {
		if match := updateTable.FindStringSubmatch(fragment); match != nil && historyUpdateLeavesColumn(match[1], column) {
			continue
		}
		if !accounted[fragment] {
			known = false
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
			insert := historyInsertColumns.FindStringSubmatch(sql)
			_, returned, ok := strings.Cut(sql, "RETURNING ")
			if insert == nil || !ok {
				continue
			}
			match = []string{"", returned, insert[1]}
		}
		columns := strings.Split(match[1], ",")
		if position < len(columns) {
			_, column, _ := strings.Cut(strings.TrimSpace(columns[position]), ".")
			if column == "" {
				column = strings.TrimSpace(columns[position])
			}
			return match[2], column, true
		}
	}
	return "", "", false
}

// Private constructor fields can carry a local closure's parameter as their key.
func (r historyImageReader) privateFieldStrings(in historyImageArg, selected *ast.SelectorExpr, depth int) (cacheValues []string, cacheKnown bool) {
	if depth > 32 {
		return nil, false
	}
	field := auditSelectedField(in.pkg.info, selected)
	if field == nil || field.Exported() {
		return nil, false
	}
	cacheKey := historyStoredCacheKey{r.index, field, 3}
	if cached, ok := historyStoredReads.Load(cacheKey); ok {
		value, valid := cached.(historyStoredCacheValue)
		if !valid {
			return nil, false
		}
		return slices.Clone(value.values), value.known
	}
	if len(r.stringFieldBusy) == 0 {
		defer func() {
			historyStoredReads.Store(cacheKey, historyStoredCacheValue{slices.Clone(cacheValues), cacheKnown})
		}()
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

// A SET form this reader cannot separate stays unknown, including tuple writes.
func historyUpdateLeavesColumn(set, column string) bool {
	for _, assignment := range strings.Split(set, ",") {
		match := historyUpdateAssignment.FindStringSubmatch(assignment)
		if match == nil || strings.EqualFold(match[1]+match[2], column) {
			return false
		}
	}
	return true
}
