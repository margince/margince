// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// How the audit write census finds Go code that writes audit_log rows without
// storekit's seed.

import (
	"go/ast"
	"go/token"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// sqlRunners are the pgx calls that run the SQL they are handed.
var sqlRunners = map[string]bool{
	"Exec": true, "ExecContext": true, "Query": true, "QueryContext": true, "QueryRow": true,
	"QueryRowContext": true, "Queue": true, "Prepare": true, "PrepareContext": true,
}

// sqlDescribers are the calls that put a string in an error or log line, which
// runs nothing.
var sqlDescribers = map[string]bool{
	"Errorf": true, "Wrap": true, "Wrapf": true, "panic": true,
	"Error": true, "Warn": true, "Info": true, "Debug": true,
	"ErrorContext": true, "WarnContext": true, "InfoContext": true, "DebugContext": true,
	"Print": true, "Printf": true, "Println": true, "Fatal": true, "Fatalf": true, "Log": true, "Logf": true,
}

// auditDirectWrites holds where Go code writes audit_log rows: through the seed,
// elsewhere, or along a path this cannot follow to a call it can classify.
type auditDirectWrites struct {
	seedSeen       bool
	others, opaque []string
}

// auditInsertTrace follows one INSERT string to the calls it reaches.
type auditInsertTrace struct {
	c        *auditCensus
	followed map[string]bool
	runs     []auditRunAt
	lost     []auditRunAt
}

// auditRunAt is where an INSERT string ends: the call that runs it, or the
// place it was lost.
type auditRunAt struct {
	src *auditSource
	fn  *ast.FuncDecl
	at  token.Pos
}

// directAuditInserts follows every string that inserts into audit_log, and
// every CopyFrom, to the call that runs it.
func directAuditInserts(t *testing.T) auditDirectWrites {
	t.Helper()
	auditWriteSites(t)
	c := auditCensusRead
	trace := &auditInsertTrace{c: c, followed: map[string]bool{}}
	for _, src := range c.sources {
		ast.PreorderStack(src.file, nil, func(n ast.Node, stack []ast.Node) bool {
			if call, isCall := n.(*ast.CallExpr); isCall && auditCalleeName(call) == "CopyFrom" {
				trace.copyFrom(src, enclosingFuncDecl(stack), call)
			}
			expr, isExpr := n.(ast.Expr)
			if !isExpr || !c.insertsIntoAuditLog(src, enclosingFuncDecl(stack), expr) {
				return true
			}
			trace.follow(src, expr, stack, 0)
			return false
		})
	}
	var out auditDirectWrites
	for _, run := range trace.runs {
		if run.src.dir == auditSeedDir && funcName(run.fn) == auditSeed {
			out.seedSeen = true
		} else {
			out.others = append(out.others, auditPosition(run.src, run.at))
		}
	}
	for _, at := range trace.lost {
		out.opaque = append(out.opaque, auditPosition(at.src, at.at))
	}
	return out
}

func auditPosition(src *auditSource, at token.Pos) string {
	return src.path + ":" + strconv.Itoa(gatekit.SourceFileSet().Position(at).Line)
}

// insertsIntoAuditLog reads a string literal or concatenation, with each named
// part taken from its constant, as SQL that inserts into audit_log. Local
// bindings are read only when a part is unread and the rest names either half,
// since following them costs the tree's every string.
func (c *auditCensus) insertsIntoAuditLog(src *auditSource, fn *ast.FuncDecl, expr ast.Expr) bool {
	text, unread, ok := c.sqlText(src, nil, expr)
	switch {
	case !ok:
		return false
	case auditInsertAnywhere.MatchString(text):
		return true
	case !unread || fn == nil || !auditInsertHalf.MatchString(text):
		return false
	}
	text, _, _ = c.sqlText(src, fn, expr)
	return auditInsertAnywhere.MatchString(text)
}

var auditInsertHalf = regexp.MustCompile(`(?i)INSERT|audit_log`)

// sqlText spells a string literal or concatenation, with " ? " for each part it
// cannot read, and reports whether it met one.
func (c *auditCensus) sqlText(src *auditSource, fn *ast.FuncDecl, expr ast.Expr) (text string, unread, ok bool) {
	var out strings.Builder
	var walk func(ast.Expr) bool
	walk = func(e ast.Expr) bool {
		switch node := e.(type) {
		case *ast.BinaryExpr:
			return node.Op == token.ADD && walk(node.X) && walk(node.Y)
		case *ast.ParenExpr:
			return walk(node.X)
		case *ast.BasicLit:
			value, err := strconv.Unquote(node.Value)
			out.WriteString(value)
			return node.Kind == token.STRING && err == nil
		case *ast.Ident, *ast.SelectorExpr:
			if values, read := c.resolve(src, fn, nil, node, 0); read && len(values) == 1 {
				out.WriteString(values[0])
			} else {
				out.WriteString(" ? ")
				unread = true
			}
			return true
		}
		return false
	}
	ok = walk(expr)
	return out.String(), unread, ok
}

// copyFrom reads the table a CopyFrom fills from its pgx.Identifier literal.
func (tr *auditInsertTrace) copyFrom(src *auditSource, fn *ast.FuncDecl, call *ast.CallExpr) {
	if len(call.Args) < 2 {
		return
	}
	if forwardsCopyTable(fn, call.Args[1]) {
		return
	}
	table, isLiteral := call.Args[1].(*ast.CompositeLit)
	if !isLiteral || len(table.Elts) == 0 {
		tr.lost = append(tr.lost, auditRunAt{src: src, fn: fn, at: call.Args[1].Pos()})
		return
	}
	names, ok := tr.c.resolve(src, fn, nil, table.Elts[len(table.Elts)-1], 0)
	if !ok {
		tr.lost = append(tr.lost, auditRunAt{src: src, fn: fn, at: table.Pos()})
		return
	}
	if slices.Contains(names, "audit_log") {
		tr.runs = append(tr.runs, auditRunAt{src: src, fn: fn, at: call.Pos()})
	}
}

// forwardsCopyTable reports whether a CopyFrom method hands on its own table
// parameter unchanged, so each call into it is the one to read.
func forwardsCopyTable(fn *ast.FuncDecl, table ast.Expr) bool {
	ident, isIdent := table.(*ast.Ident)
	if fn == nil || fn.Name.Name != "CopyFrom" || !isIdent || auditParamIndex(fn, ident.Name) < 0 {
		return false
	}
	_, assigned := localValues(fn, ident.Name)
	return !assigned
}

// follow climbs from a use of the SQL through concatenation and formatting. It
// stops at the call that runs or describes it, or at the name it is bound to.
func (tr *auditInsertTrace) follow(src *auditSource, use ast.Node, stack []ast.Node, depth int) {
	for i := len(stack) - 1; i >= 0 && depth <= auditResolveDepth; i-- {
		switch parent := stack[i].(type) {
		case *ast.ParenExpr:
			use = parent
			continue
		case *ast.BinaryExpr:
			if parent.Op == token.ADD {
				use = parent
				continue
			}
		case *ast.CallExpr:
			name := auditCalleeName(parent)
			switch {
			case parent.Fun == use:
			case sqlRunners[name]:
				tr.runs = append(tr.runs, auditRunAt{src: src, fn: enclosingFuncDecl(stack[:i]), at: parent.Pos()})
				return
			case sqlDescribers[name] || isErrorsNew(parent):
				return
			case name == "Sprintf":
				use = parent
				continue
			}
		case *ast.AssignStmt, *ast.ValueSpec:
			if name, bound := boundName(parent, use); bound {
				tr.followName(src, enclosingFuncDecl(stack[:i]), name, depth+1)
				return
			}
		}
		break
	}
	tr.lost = append(tr.lost, auditRunAt{src: src, fn: enclosingFuncDecl(stack), at: use.Pos()})
}

func isErrorsNew(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && isIdentNamed(sel.X, "errors") && sel.Sel.Name == "New"
}

// boundName is the name an assignment or declaration binds value to.
func boundName(binding ast.Node, value ast.Node) (string, bool) {
	var names, values []ast.Expr
	switch b := binding.(type) {
	case *ast.AssignStmt:
		names, values = b.Lhs, b.Rhs
	case *ast.ValueSpec:
		values = b.Values
		for _, ident := range b.Names {
			names = append(names, ident)
		}
	}
	at := slices.IndexFunc(values, func(v ast.Expr) bool { return v == value })
	if at < 0 || len(names) != len(values) {
		return "", false
	}
	ident, ok := names[at].(*ast.Ident)
	if !ok || ident.Name == "_" {
		return "", false
	}
	return ident.Name, true
}

// followName follows every read of a name holding the SQL: in fn for a local,
// or across its package and importers for a package-level name.
func (tr *auditInsertTrace) followName(src *auditSource, fn *ast.FuncDecl, name string, depth int) {
	key := src.dir + "." + funcName(fn) + "." + name
	if tr.followed[key] {
		return
	}
	tr.followed[key] = true
	if fn != nil {
		tr.followReads(src, fn, name, "", depth)
		return
	}
	for _, other := range tr.c.sources {
		if other.dir == src.dir {
			tr.followReads(other, other.file, name, "", depth)
		}
		for qualifier, dir := range other.imports {
			if dir == src.dir {
				tr.followReads(other, other.file, name, qualifier, depth)
			}
		}
	}
}

// followReads follows each read of name under root, as qualifier.name when a
// qualifier is given.
func (tr *auditInsertTrace) followReads(src *auditSource, root ast.Node, name, qualifier string, depth int) {
	ast.PreorderStack(root, nil, func(n ast.Node, stack []ast.Node) bool {
		var read ast.Node
		switch node := n.(type) {
		case *ast.SelectorExpr:
			if qualifier != "" && isIdentNamed(node.X, qualifier) && node.Sel.Name == name {
				read = node
			}
		case *ast.Ident:
			if qualifier == "" && node.Name == name && !bindsOrNames(stack[len(stack)-1], node) {
				read = node
			}
		}
		if read == nil {
			return true
		}
		tr.follow(src, read, stack, depth)
		return false
	})
}

// bindsOrNames reports whether ident under parent declares or assigns a name,
// or names a field or selector, rather than reading a value.
func bindsOrNames(parent ast.Node, ident *ast.Ident) bool {
	switch p := parent.(type) {
	case *ast.SelectorExpr:
		return p.Sel == ident
	case *ast.KeyValueExpr:
		return p.Key == ident
	case *ast.AssignStmt:
		return slices.Contains(p.Lhs, ast.Expr(ident))
	case *ast.ValueSpec:
		return slices.Contains(p.Names, ident)
	case *ast.RangeStmt:
		return p.Key == ident || p.Value == ident
	case *ast.Field, *ast.LabeledStmt, *ast.BranchStmt:
		return true
	}
	return false
}
