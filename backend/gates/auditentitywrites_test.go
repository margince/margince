// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// SQL that writes audit_log outside storekit's seed, which the census would
// not follow.

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"regexp"
	"slices"
	"strings"
)

// auditWriteVerb is a SQL verb that adds audit rows, up to the table's name.
// An UPDATE is read apart, by whether it sets entity_type.
const auditWriteVerb = `(?is)\b(?:INSERT\s+INTO|MERGE\s+INTO|COPY)\s+(?:ONLY\s+)?(?:"?public"?\.)?"?`

var (
	// auditWriteSQL finds a write to audit_log in a constant's value.
	auditWriteSQL = regexp.MustCompile(auditWriteVerb + `audit_log\b`)
	// auditWriteOpen finds SQL that ends where its write's table is spliced in.
	auditWriteOpen = regexp.MustCompile(auditWriteVerb + `$`)
	// auditWriteFormat finds a format string that names its write's table
	// through a verb.
	auditWriteFormat = regexp.MustCompile(auditWriteVerb + `%`)
	// auditUpdate finds an UPDATE of audit_log, and auditSetEnds where its SET
	// list ends.
	auditUpdate     = regexp.MustCompile(`(?is)\bUPDATE\s+(?:ONLY\s+)?(?:"?public"?\.)?"?audit_log\b[^;]*?\bSET\b`)
	auditSetEnds    = regexp.MustCompile(`(?is)\b(?:FROM|WHERE|RETURNING)\b|;`)
	auditSetsEntity = regexp.MustCompile(`(?is)\bentity_type\s*=`)
)

// writesAuditRecordType reports whether SQL adds an audit row or changes the
// record type of one. An UPDATE that rewrites only the images names no type.
func writesAuditRecordType(sql string) bool {
	if auditWriteSQL.MatchString(sql) {
		return true
	}
	for _, at := range auditUpdate.FindAllStringIndex(sql, -1) {
		set := sql[at[1]:]
		if end := auditSetEnds.FindStringIndex(set); end != nil {
			set = set[:end[0]]
		}
		if auditSetsEntity.MatchString(set) {
			return true
		}
	}
	return false
}

// directAuditInserts finds each write to audit_log outside the seed. That is
// a constant holding one, SQL that splices, formats or joins "audit_log" in as
// its table, or a pgx.Identifier naming the table. The type
// checker folds a concatenation of constants, so the outermost constant
// expression is the one read.
func directAuditInserts(index *auditIndex, seed string) (found []string, seedInserts bool) {
	for i := range index.pkgs {
		pkg := &index.pkgs[i]
		for _, file := range pkg.files {
			for _, decl := range file.Decls {
				fn, _ := decl.(*ast.FuncDecl)
				r := &auditResolver{index: index, pkg: pkg, decl: fn, visiting: map[types.Object]bool{}}
				inserts := r.directInserts(decl)
				if fn != nil && auditFuncKey(pkg.info, fn) == seed {
					seedInserts = len(inserts) > 0
					continue
				}
				found = append(found, inserts...)
			}
		}
	}
	return found, seedInserts
}

func (r *auditResolver) directInserts(decl ast.Decl) []string {
	var found []string
	info := r.pkg.info
	ast.Inspect(decl, func(n ast.Node) bool {
		expr, ok := n.(ast.Expr)
		if !ok {
			return true
		}
		if value, isConst := auditConstString(info, expr); isConst {
			// A constant's name is read where its value is written, not at
			// each use.
			if _, named := ast.Unparen(expr).(*ast.Ident); named {
				return false
			}
			if _, named := ast.Unparen(expr).(*ast.SelectorExpr); named {
				return false
			}
			if writesAuditRecordType(value) {
				found = append(found, auditRepoPos(expr.Pos()))
			}
			return false
		}
		if r.splicesAuditLog(expr) || writesAuditRecordType(r.evaluated(expr)) {
			found = append(found, auditRepoPos(expr.Pos()))
		}
		return true
	})
	return found
}

// splicesAuditLog reports whether expr builds a write, or a table name,
// from a value that can be "audit_log".
func (r *auditResolver) splicesAuditLog(expr ast.Expr) bool {
	info := r.pkg.info
	names := func(e ast.Expr) bool {
		read, _ := r.resolve(e)
		return slices.Contains(read.values, storekitTableAudit)
	}
	switch node := expr.(type) {
	case *ast.BinaryExpr:
		left, ok := auditConstString(info, node.X)
		return node.Op == token.ADD && ok && auditWriteOpen.MatchString(left) && names(node.Y)
	case *ast.CallExpr:
		formatted := false
		for _, arg := range node.Args {
			if value, ok := auditConstString(info, arg); ok && auditWriteFormat.MatchString(value) {
				formatted = true
			} else if formatted && names(arg) {
				return true
			}
		}
	case *ast.CompositeLit:
		if tv, ok := info.Types[node]; ok && types.TypeString(tv.Type, nil) == "github.com/jackc/pgx/v5.Identifier" {
			return slices.ContainsFunc(node.Elts, names)
		}
	}
	return false
}

// evaluated is the text a call to fmt.Sprintf or strings.Join builds from
// constants only, or "".
func (r *auditResolver) evaluated(expr ast.Expr) string {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return ""
	}
	callee := auditCallee(r.pkg.info, call)
	if callee == nil {
		return ""
	}
	switch callee.FullName() {
	case "fmt.Sprintf":
		args, ok := r.constantArgs(call.Args)
		if !ok || len(args) == 0 {
			return ""
		}
		format, isString := args[0].(string)
		if !isString {
			return ""
		}
		return fmt.Sprintf(format, args[1:]...)
	case "strings.Join":
		lit, isLit := ast.Unparen(call.Args[0]).(*ast.CompositeLit)
		sep, isConst := auditConstString(r.pkg.info, call.Args[1])
		if !isLit || !isConst {
			return ""
		}
		parts, ok := r.constantArgs(lit.Elts)
		if !ok {
			return ""
		}
		words := make([]string, len(parts))
		for i, part := range parts {
			words[i] = fmt.Sprint(part)
		}
		return strings.Join(words, sep)
	}
	return ""
}

func (r *auditResolver) constantArgs(exprs []ast.Expr) ([]any, bool) {
	out := make([]any, 0, len(exprs))
	for _, expr := range exprs {
		tv, ok := r.pkg.info.Types[expr]
		if !ok || tv.Value == nil {
			return nil, false
		}
		out = append(out, constant.Val(tv.Value))
	}
	return out, true
}
