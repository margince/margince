// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A census over the censuses: whoever judges this tree's SQL must read it the
// way Postgres receives it.
//
// `ast.BasicLit.Value` is SOURCE TEXT. For a backticked literal the text and
// the string are the same, and 5,314 of this tree's SQL literals are written
// that way — which is why no census reading `.Value` has ever reported the
// difference. Nothing makes SQL backticked here. The day somebody writes one
// statement in double quotes, every census reading raw text sees `\n` as a
// backslash and an `n`: a pattern asking for `\s` matches nothing, a reader
// splitting on "\n" gets one unbroken line, and the census reports a clean
// tree. That is the failure a census exists to prevent, standing inside the
// census.
//
// So the reading lives once, in gatekit, and this is what stops the thirteenth
// census being written raw again. A reader here either goes through gatekit or
// decodes with strconv.Unquote — the second is correct on this axis and is
// what the tree already does in a dozen places, so it is admitted rather than
// swept.
//
// Judged over the files that JUDGE SQL. A literal reader that never looks at a
// statement — an import path, an event name, a scope constant — cannot be bitten
// by a statement written the other way, and dragging those in would make this a
// waiver list rather than a finding.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// judgesSQL matches a file that has SQL vocabulary in it — the census's subject
// is the readers that judge statements, not every reader of a Go literal.
var judgesSQL = regexp.MustCompile(`(?i)(SELECT\s|INSERT\s+INTO|UPDATE\s|DELETE\s+FROM|\bFROM\s|\bJOIN\s)`)

// sqlLiteralReaderFloor is what the walk found when this census landed. A
// census that can fail SHORT has already failed: pinned, a walk that stops
// reaching the readers says so instead of reading green over nothing.
const sqlLiteralReaderFloor = 20

// rawReaderWaivers ratifies the files where the SOURCE text is what is meant.
// Each is checked, and AssertAllMatched reports one that has gone stale.
var rawReaderWaivers = gatekit.Waive(map[string]string{
	"gates/inboundsigningrecipe_test.go": "compares a Go format against one read out of .tsx TEXT, where the separator really is a backslash and an n — decoded, this side would be a newline and the two would read as disagreeing",
})

// judgesSQLLiterals is the census's subject: a file that both walks Go string
// literals and has SQL vocabulary of its own. A reader that never looks at a
// statement — an import path, an event name, a scope constant — cannot be
// bitten by a statement written the other way, and sweeping those in would make
// this a waiver list rather than a finding.
func judgesSQLLiterals(file *ast.File) bool {
	if !walksLiterals(file) {
		return false
	}
	for _, text := range gatekit.SQLStatementsOf(file) {
		if judgesSQL.MatchString(text) {
			return true
		}
	}
	return false
}

// walksLiterals reports whether the file names *ast.BasicLit at all.
func walksLiterals(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		if expr, isExpr := node.(ast.Expr); isExpr && isBasicLitType(expr) {
			found = true
		}
		return !found
	})
	return found
}

func TestEveryCensusOfSQLReadsItAsPostgresReceivesIt(t *testing.T) {
	t.Parallel()
	var findings []string
	var judged []string
	fset := token.NewFileSet()
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "node_modules" || entry.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			t.Errorf("parsing %s: %v", path, parseErr)
			return nil
		}
		if !judgesSQLLiterals(file) {
			return nil
		}
		judged = append(judged, filepath.ToSlash(path))
		if rawReaderWaivers.Waived(t, filepath.ToSlash(path)) {
			return nil
		}
		for _, site := range rawLiteralReadsIn(file) {
			findings = append(findings, filepath.ToSlash(path)+": "+site)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module: %v", err)
	}
	// A census that can fail short has already failed: a walk that stopped
	// finding readers would report a clean tree in the same words as a tree
	// with none left to fix.
	if len(judged) < sqlLiteralReaderFloor {
		t.Fatalf("only %d file(s) judge SQL literals, and this census is pinned at %d:\n\t%s\n\n"+
			"A walk that stopped reaching them reports a clean tree in the same words as a tree "+
			"with nothing left to fix. Lower the floor deliberately, or find what the walk lost.",
			len(judged), sqlLiteralReaderFloor, strings.Join(judged, "\n\t"))
	}
	rawReaderWaivers.AssertAllMatched(t)
	if len(findings) > 0 {
		t.Errorf("%d SQL literal read(s) take the source text rather than the string:\n\t%s\n\n"+
			"A statement written in double quotes reaches these as its escapes, so the census "+
			"reports a clean tree over the very shape it exists to find. Read it with "+
			"gatekit.SQLStatementsOf (statements, escapes decoded, `+` chains flattened), "+
			"gatekit.LiteralText (one literal), or strconv.Unquote.",
			len(findings), strings.Join(findings, "\n\t"))
	}
}

// rawLiteralReadsIn names every `.Value` read of a *ast.BasicLit in the file
// that is not handed straight to a decoder or a message.
//
// Bindings are collected by SHAPE rather than by type-checking: the three ways
// this tree binds a literal are a type assertion, a type switch and a
// parameter, and a census that needed full type information to run would not
// run in a test.
//
// Scoped per type-switch CLAUSE, not per switch. `switch n := node.(type)` gives
// the same name a different type in every arm, so a set collected over the whole
// switch reports `n.Value` in the *ast.RangeStmt arm — which is that node's own
// Value field and nothing to do with a literal. Reported once, a finding like
// that teaches its reader that this gate does not know what it is looking at.
func rawLiteralReadsIn(file *ast.File) []string {
	var out []string
	readLiteralValues(file, map[string]bool{}, &out)
	return out
}

func readLiteralValues(node ast.Node, bound map[string]bool, out *[]string) {
	switch typed := node.(type) {
	case nil:
		return
	case *ast.CallExpr:
		if decodesALiteral(typed) || reportsALiteral(typed) {
			// Its arguments are this read: decoded, or printed. Neither is a
			// match against the source text.
			return
		}
	case *ast.TypeSwitchStmt:
		readTypeSwitchValues(typed, bound, out)
		return
	case *ast.FuncDecl:
		readLiteralValues(typed.Body, withLiteralParams(bound, typed.Type), out)
		return
	case *ast.FuncLit:
		readLiteralValues(typed.Body, withLiteralParams(bound, typed.Type), out)
		return
	case *ast.BlockStmt:
		// Statements in order, carrying each binding forward: a literal bound
		// on one line is read on the next, and a walk that scoped the binding
		// to the assignment itself would see none of them.
		for _, statement := range typed.List {
			readLiteralValues(statement, bound, out)
			if assign, isAssign := statement.(*ast.AssignStmt); isAssign {
				bound = withLiteralAssignment(bound, assign)
			}
		}
		return
	case *ast.IfStmt:
		// `if lit, ok := expr.(*ast.BasicLit); ok { … }` binds for the whole
		// statement, which is where this tree writes most of its literal reads.
		bound = withStatementBinding(bound, typed.Init)
		readLiteralValues(typed.Init, bound, out)
		readLiteralValues(typed.Cond, bound, out)
		readLiteralValues(typed.Body, bound, out)
		readLiteralValues(typed.Else, bound, out)
		return
	case *ast.SwitchStmt:
		bound = withStatementBinding(bound, typed.Init)
		readLiteralValues(typed.Init, bound, out)
		readLiteralValues(typed.Tag, bound, out)
		readLiteralValues(typed.Body, bound, out)
		return
	case *ast.ForStmt:
		bound = withStatementBinding(bound, typed.Init)
		readLiteralValues(typed.Init, bound, out)
		readLiteralValues(typed.Cond, bound, out)
		readLiteralValues(typed.Post, bound, out)
		readLiteralValues(typed.Body, bound, out)
		return
	case *ast.CaseClause:
		for _, statement := range typed.Body {
			readLiteralValues(statement, bound, out)
			if assign, isAssign := statement.(*ast.AssignStmt); isAssign {
				bound = withLiteralAssignment(bound, assign)
			}
		}
		for _, expr := range typed.List {
			readLiteralValues(expr, bound, out)
		}
		return
	case *ast.SelectorExpr:
		if typed.Sel.Name == "Value" {
			if ident, isIdent := typed.X.(*ast.Ident); isIdent && bound[ident.Name] {
				*out = append(*out, ident.Name+".Value")
			}
		}
	}
	for _, child := range childrenOf(node) {
		readLiteralValues(child, bound, out)
	}
}

// readTypeSwitchValues walks each arm with the switch's name bound only where
// that arm's type is the literal.
func readTypeSwitchValues(stmt *ast.TypeSwitchStmt, bound map[string]bool, out *[]string) {
	readLiteralValues(stmt.Assign, bound, out)
	name := typeSwitchName(stmt)
	for _, clause := range stmt.Body.List {
		caseClause, isCase := clause.(*ast.CaseClause)
		if !isCase {
			continue
		}
		armBound := bound
		if name != "" {
			armBound = copyBindings(bound)
			armBound[name] = clauseBindsLiteral(caseClause)
		}
		for _, statement := range caseClause.Body {
			readLiteralValues(statement, armBound, out)
			if assign, isAssign := statement.(*ast.AssignStmt); isAssign {
				armBound = withLiteralAssignment(armBound, assign)
			}
		}
	}
}

func typeSwitchName(stmt *ast.TypeSwitchStmt) string {
	assign, isAssign := stmt.Assign.(*ast.AssignStmt)
	if !isAssign || len(assign.Lhs) != 1 {
		return ""
	}
	if ident, isIdent := assign.Lhs[0].(*ast.Ident); isIdent {
		return ident.Name
	}
	return ""
}

func clauseBindsLiteral(clause *ast.CaseClause) bool {
	for _, typ := range clause.List {
		if isBasicLitType(typ) {
			return true
		}
	}
	return false
}

// withStatementBinding extends the bindings if the statement is an assignment.
func withStatementBinding(bound map[string]bool, stmt ast.Stmt) map[string]bool {
	if assign, isAssign := stmt.(*ast.AssignStmt); isAssign {
		return withLiteralAssignment(bound, assign)
	}
	return bound
}

func withLiteralAssignment(bound map[string]bool, assign *ast.AssignStmt) map[string]bool {
	if len(assign.Rhs) != 1 || len(assign.Lhs) == 0 {
		return bound
	}
	assertion, isAssertion := assign.Rhs[0].(*ast.TypeAssertExpr)
	if !isAssertion {
		return bound
	}
	name, isIdent := assign.Lhs[0].(*ast.Ident)
	if !isIdent {
		return bound
	}
	next := copyBindings(bound)
	next[name.Name] = isBasicLitType(assertion.Type)
	return next
}

func withLiteralParams(bound map[string]bool, sig *ast.FuncType) map[string]bool {
	if sig == nil || sig.Params == nil {
		return bound
	}
	next := copyBindings(bound)
	for _, param := range sig.Params.List {
		literal := isBasicLitType(param.Type)
		for _, name := range param.Names {
			next[name.Name] = literal
		}
	}
	return next
}

func copyBindings(bound map[string]bool) map[string]bool {
	next := make(map[string]bool, len(bound)+1)
	for name, isLiteral := range bound {
		next[name] = isLiteral
	}
	return next
}

// childrenOf is ast.Inspect's descent, one level, so the walk above can carry
// its own scope instead of a single set over the whole file.
func childrenOf(node ast.Node) []ast.Node {
	var kids []ast.Node
	first := true
	ast.Inspect(node, func(child ast.Node) bool {
		if first {
			first = false
			return true
		}
		if child != nil {
			kids = append(kids, child)
		}
		return false
	})
	return kids
}

// decodesALiteral reports whether the call is one of the readings that yields
// the string rather than the source text.
func decodesALiteral(call *ast.CallExpr) bool {
	selector, isSelector := call.Fun.(*ast.SelectorExpr)
	if !isSelector {
		return false
	}
	switch selector.Sel.Name {
	case "Unquote", "LiteralText", "TextOf", "SQLStatementsIn", "SQLStatementsOf", "SQLTextOf", "ConcatenatedString":
		return true
	}
	return false
}

// reportsALiteral reports whether the call PRINTS its arguments rather than
// matching them. The source text is the right thing to put in a failure — it is
// what the author will search for — so a read that only ever reaches a message
// is not this census's business.
func reportsALiteral(call *ast.CallExpr) bool {
	name := ""
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		name = fun.Sel.Name
	case *ast.Ident:
		name = fun.Name
	}
	switch name {
	case "Errorf", "Fatalf", "Logf", "Skipf", "Printf", "Sprintf", "Error", "Fatal", "Log", "Sprint", "Sprintln", "Println":
		return true
	}
	return false
}

// isBasicLitType matches `*ast.BasicLit`, which is how this tree spells it.
func isBasicLitType(expr ast.Expr) bool {
	star, isStar := expr.(*ast.StarExpr)
	if !isStar {
		return false
	}
	selector, isSelector := star.X.(*ast.SelectorExpr)
	if !isSelector || selector.Sel.Name != "BasicLit" {
		return false
	}
	pkg, isIdent := selector.X.(*ast.Ident)
	return isIdent && pkg.Name == "ast"
}

// The detector, from the other end. A census over censuses that stopped seeing
// a raw read would report a clean tree in the same words as a tree with none —
// so each shape this shape is written in is planted here and must be found.
func TestTheReaderCensusSeesEachShapeARawReadIsWrittenIn(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		source string
		want   int
	}{
		{
			name: "a type assertion binds the literal",
			source: "package p\nimport \"go/ast\"\nfunc f(n ast.Node) string {\n" +
				"\tlit, ok := n.(*ast.BasicLit)\n\tif !ok {\n\t\treturn \"\"\n\t}\n\treturn lit.Value\n}\n",
			want: 1,
		},
		{
			name: "a type switch arm binds the literal",
			source: "package p\nimport \"go/ast\"\nfunc f(n ast.Node) string {\n" +
				"\tswitch v := n.(type) {\n\tcase *ast.BasicLit:\n\t\treturn v.Value\n\t}\n\treturn \"\"\n}\n",
			want: 1,
		},
		{
			name:   "a parameter binds the literal",
			source: "package p\nimport \"go/ast\"\nfunc f(lit *ast.BasicLit) string {\n\treturn lit.Value\n}\n",
			want:   1,
		},
		{
			// The finding this census exists to NOT report. Every arm of a type
			// switch shares one name, and the other arms' Value fields are their
			// own.
			name: "another arm's Value field is not a literal read",
			source: "package p\nimport \"go/ast\"\nfunc f(n ast.Node) ast.Expr {\n" +
				"\tswitch v := n.(type) {\n\tcase *ast.KeyValueExpr:\n\t\treturn v.Value\n\t}\n\treturn nil\n}\n",
			want: 0,
		},
		{
			name: "an if-statement init binds the literal",
			source: "package p\nimport \"go/ast\"\nfunc f(e ast.Expr) string {\n" +
				"\tif lit, ok := e.(*ast.BasicLit); ok {\n\t\treturn lit.Value\n\t}\n\treturn \"\"\n}\n",
			want: 1,
		},
		{
			name: "a decoded read is not a raw one",
			source: "package p\nimport (\n\t\"go/ast\"\n\t\"strconv\"\n)\nfunc f(lit *ast.BasicLit) string {\n" +
				"\ttext, _ := strconv.Unquote(lit.Value)\n\treturn text\n}\n",
			want: 0,
		},
		{
			name: "a read that only reaches a message is not a match",
			source: "package p\nimport (\n\t\"go/ast\"\n\t\"testing\"\n)\nfunc f(t *testing.T, lit *ast.BasicLit) {\n" +
				"\tt.Errorf(\"unreadable: %s\", lit.Value)\n}\n",
			want: 0,
		},
		{
			name: "the shared reader is not a raw read",
			source: "package p\nimport (\n\t\"go/ast\"\n\t\"github.com/margince/margince/backend/internal/shared/gatekit\"\n)\n" +
				"func f(lit *ast.BasicLit) string {\n\treturn gatekit.TextOf(lit)\n}\n",
			want: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parsed, err := parser.ParseFile(token.NewFileSet(), "probe.go", tc.source, 0)
			if err != nil {
				t.Fatalf("parsing the probe: %v", err)
			}
			if got := rawLiteralReadsIn(parsed); len(got) != tc.want {
				t.Errorf("the detector found %d raw read(s), want %d: %v", len(got), tc.want, got)
			}
		})
	}
}
