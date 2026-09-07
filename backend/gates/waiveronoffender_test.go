// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A waiver must be asked about an offender, never about a candidate.
//
// gatekit.Waived records matched[subject] and AssertAllMatched fails an entry
// that was never matched. Asked on the OFFENDER path that makes every waiver
// bidirectional for free: fix the subject, the loop stops reaching Waived, the
// entry goes stale, the gate says so. Guarding four archive verbs in #2142
// immediately failed their four unguardedByIDUpdates entries with "matched no
// subject" — nobody had to remember to delete them.
//
// Asked as a PRE-FILTER, that property is gone, and it fails in the worse
// direction too: the guard discards a whole SET of potential findings before
// any of them is determined. A new offence added to a waived file is then
// invisible to the gate whose entire purpose is to find it — green, with the
// rule unenforced for exactly that file. That is what #2164 found in the
// restricted-reader gate, and this test is the half that keeps the next one
// from being written the same way.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// guardFloor pins the census. A walk that stopped recognising Waived guards
// would report a clean tree in the same words as a tree with nothing to fix.
const guardFloor = 60

// prefilterAdmitted ratifies the guards that discard a set and are still on the
// offender path, because the set collapses to one finding.
//
// Keyed by file and line-bearing subject rather than by file, for the reason
// this whole test exists.
var prefilterAdmitted = gatekit.Waive(map[string]string{
	"gates/jobfleetwide_test.go:fleetWideArgsType(d.args)":        "the subject IS the offender — one dispatcher — and the set the guard skips is that one dispatcher's own writes, every finding naming it. It is also the one waiver here that already checks itself in the other direction: a dispatcher waived as doing its own pass whose Work DOES fan out is reported rather than skipped, so the entry cannot quietly outlive the code it describes. The cost is that a second offence added to a waived dispatcher is covered by its entry, which is the grain the entry is written at",
	"internal/modules/privacy/activityerasureparity_test.go:name": "the subject IS the offender — one destroyer that does not finish the erasure — and the set the guard skips is the callers OF that destroyer, each finding naming it. A caller is not a separate offence here: the destroyer is what is waived, and a waiver of it is a statement about every path that reaches it. The cost is that this entry must be reread if the gate ever grows a finding about a caller in its own right",
})

// TestEveryWaiverIsAskedAboutAnOffenderNotACandidate is the gate over the gates.
//
// The rule is syntactic on purpose. A guard is a pre-filter when both hold:
// the code it short-circuits past still has findings to DETERMINE — a loop that
// reports or collects, or a slice of findings spread into one — and those
// findings name something the waiver's subject does not. Either alone is
// innocent: a guard that skips one finding discards one, and a guard whose
// findings name only what its subject names discards one finding written many
// times. Together they are a waiver answering for a category.
func TestEveryWaiverIsAskedAboutAnOffenderNotACandidate(t *testing.T) {
	t.Parallel()
	defer prefilterAdmitted.AssertAllMatched(t)

	fset := token.NewFileSet()
	guards := 0
	var findings []string
	for _, path := range waiverCensusFiles(t) {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			block, ok := n.(*ast.BlockStmt)
			if !ok {
				return true
			}
			for i, stmt := range block.List {
				guard, subject := waiverGuard(stmt)
				if guard == nil {
					continue
				}
				guards++
				skipped := block.List[i+1:]
				if !discardsASet(skipped) {
					continue
				}
				unnamed := findingsNameBeyond(skipped, identsIn(subject))
				if len(unnamed) == 0 {
					continue
				}
				key := path + ":" + waiverSubjectText(fset, subject)
				if prefilterAdmitted.Waived(t, key) {
					continue
				}
				findings = append(findings, path+":"+waiverLineNumber(fset.Position(guard.Pos()).Line)+
					": this waiver is asked about a candidate, not an offender — the code it skips still has "+
					strings.Join(unnamed, ", ")+" to determine, so the entry answers for every offence in "+
					"that set including ones written after it, and AssertAllMatched can no longer report it "+
					"stale. Move the Waived call inside the loop that produces the offence and key it on the "+
					"offence, as restrictedreaders_test.go and edgereaders_test.go do")
			}
			return true
		})
	}
	if guards < guardFloor {
		t.Fatalf("this census found %d Waived guard(s) and is pinned at %d — it has stopped recognising "+
			"them rather than the tree having lost them", guards, guardFloor)
	}
	sort.Strings(findings)
	for _, finding := range findings {
		t.Error(finding)
	}
	t.Logf("Waived guards judged: %d", guards)
}

// waiverGuard returns the `if …Waived(t, subject) { continue }` at stmt, and
// the subject it asks about. A guard that does not short-circuit cannot skip
// anything, so it is not one.
func waiverGuard(stmt ast.Stmt) (*ast.IfStmt, ast.Expr) {
	ifStmt, ok := stmt.(*ast.IfStmt)
	if !ok || !shortCircuits(ifStmt.Body) {
		return nil, nil
	}
	var subject ast.Expr
	ast.Inspect(ifStmt.Cond, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 2 {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Waived" {
			subject = call.Args[1]
			return false
		}
		return true
	})
	if subject == nil {
		return nil, nil
	}
	return ifStmt, subject
}

func shortCircuits(body *ast.BlockStmt) bool {
	if body == nil {
		return false
	}
	for _, stmt := range body.List {
		switch s := stmt.(type) {
		case *ast.BranchStmt:
			if s.Tok == token.CONTINUE || s.Tok == token.BREAK {
				return true
			}
		case *ast.ReturnStmt:
			return true
		}
	}
	return false
}

// discardsASet reports whether the skipped code still has more than one finding
// to determine: a loop that reports or collects, or a slice of findings spread
// into one with append(…, f(…)...).
func discardsASet(skipped []ast.Stmt) bool {
	set := false
	for _, stmt := range skipped {
		ast.Inspect(stmt, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.RangeStmt:
				set = set || len(recordings(x.Body.List)) > 0
			case *ast.ForStmt:
				set = set || len(recordings(x.Body.List)) > 0
			case *ast.CallExpr:
				if fn, ok := x.Fun.(*ast.Ident); ok && fn.Name == "append" && x.Ellipsis != token.NoPos {
					set = true
				}
			}
			return true
		})
	}
	return set
}

// recordings are the expressions a finding is written from: what a t.Errorf
// family call is given, and what is appended to a findings slice.
func recordings(stmts []ast.Stmt) []ast.Expr {
	var out []ast.Expr
	for _, stmt := range stmts {
		ast.Inspect(stmt, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fn := call.Fun.(type) {
			case *ast.SelectorExpr:
				switch fn.Sel.Name {
				case "Errorf", "Error", "Fatalf", "Fatal":
					out = append(out, call.Args...)
				}
			case *ast.Ident:
				if fn.Name == "append" && len(call.Args) > 1 {
					out = append(out, call.Args[1:]...)
				}
			}
			return true
		})
	}
	return out
}

// findingsNameBeyond returns what the skipped findings name that the waiver's
// subject does not — the part of the offence the entry cannot be about.
func findingsNameBeyond(skipped []ast.Stmt, subject map[string]bool) []string {
	var beyond []string
	seen := map[string]bool{}
	for _, expr := range append(recordingsDeep(skipped), recordings(skipped)...) {
		for name := range identsIn(expr) {
			// `t` is the testing handle and `err` is the failure being read
			// out; neither is part of the offence's identity.
			if subject[name] || name == "t" || name == "err" || seen[name] {
				continue
			}
			seen[name] = true
			beyond = append(beyond, name)
		}
	}
	sort.Strings(beyond)
	return beyond
}

func recordingsDeep(stmts []ast.Stmt) []ast.Expr {
	var out []ast.Expr
	for _, stmt := range stmts {
		ast.Inspect(stmt, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.RangeStmt:
				out = append(out, recordingsDeep(x.Body.List)...)
				out = append(out, recordings(x.Body.List)...)
			case *ast.ForStmt:
				out = append(out, recordingsDeep(x.Body.List)...)
				out = append(out, recordings(x.Body.List)...)
			}
			return true
		})
	}
	return out
}

// identsIn collects the names that carry an expression's grain: the values it
// is built from, not the functions it calls or the packages they live in.
// `path+":"+fn.Name.Name` names path and fn; `fleetWideArgsType(d.args)` names d.
func identsIn(expr ast.Node) map[string]bool {
	out := map[string]bool{}
	var walk func(ast.Node)
	walk = func(n ast.Node) {
		switch x := n.(type) {
		case nil:
			return
		case *ast.CallExpr:
			for _, arg := range x.Args {
				walk(arg)
			}
			return
		case *ast.SelectorExpr:
			walk(x.X)
			return
		case *ast.Ident:
			out[x.Name] = true
			return
		}
		ast.Inspect(n, func(m ast.Node) bool {
			if m == nil || m == n {
				return m == n
			}
			walk(m)
			return false
		})
	}
	walk(expr)
	return out
}

func waiverSubjectText(fset *token.FileSet, expr ast.Expr) string {
	start := fset.Position(expr.Pos())
	end := fset.Position(expr.End())
	source, err := os.ReadFile(start.Filename)
	if err != nil || end.Offset > len(source) {
		return start.Filename
	}
	return string(source[start.Offset:end.Offset])
}

func waiverLineNumber(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// waiverCensusFiles walks the module for Go files, tests INCLUDED — the
// sibling walkers here skip _test.go, and a gate is a test, so a census of
// gates that reused one of them would read an empty tree.
func waiverCensusFiles(t *testing.T) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(".", func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case "node_modules", "testdata", "vendor", ".git":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			paths = append(paths, filepath.ToSlash(path))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module: %v", err)
	}
	sort.Strings(paths)
	return paths
}
