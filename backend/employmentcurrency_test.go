// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package backendarch

// people.EmploymentIsCurrentSQL calls itself "the ONE spelling of 'this job is
// still theirs', and the only definition of a current employment in this
// product". That was a claim with nothing holding it, and it was false eleven
// times over.
//
// Eight statements asked whether an employment was current with a bare
// `ended_at IS NULL`, which is exactly the defect the helper's own comment
// describes: somebody serving three months' notice still works there, and
// reading the column's mere presence as "gone" took them off their employer's
// contact list the day their notice was filed. Three more hand-spelled the
// correct form, and one of those compared against a Go clock instead of the
// database's, in the same statement as a half that used the database's — so a
// single query asked its two questions on two different days whenever the
// server and Postgres disagreed about the date.
//
// This is what holds the claim now. It judges STATEMENTS that ask about an
// employment: a SQL literal naming `kind = 'employment'` (or joining the
// relationship table under an employment predicate) must not decide currency
// by testing `ended_at` itself. It must call the helper.
//
// What it deliberately does NOT judge: a relationship of another kind. A
// `deal_stakeholder` or a `partner_of` edge also carries `ended_at`, and
// whether a future end date leaves one of those current is a different
// question that nobody has answered yet. Widening this gate to cover them would
// be asserting an answer rather than holding one.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/shared/gatekit"
)

// blockedByTheModuleDAG ratifies the statements that cannot adopt the helper
// TODAY, each with the reason it cannot — which is the same reason in every
// case and is architectural, not a matter of somebody not getting round to it.
//
// EmploymentIsCurrentSQL lives in modules/people, and a module never imports a
// sibling (ADR-0054 §3). compose may reach it and does; people's own files
// reach it directly; three sibling modules cannot — FIVE statements across
// activities, projects and signals, since resolver.go carries two — and the
// predicate would have to move tier before they could. That is an architecture decision with an
// owner, so it is an issue rather than a change smuggled into this one — margince/margince#2360.
//
// Each entry is a FILE and not the whole module, so a new statement in one of
// these packages is still a finding — the ratification covers the sites that
// exist, not the topic.
var blockedByTheModuleDAG = gatekit.Waive(map[string]string{
	"internal/modules/activities/orgscope.go": "activities cannot import people (ADR-0054 §3); the predicate must move tier first",
	"internal/modules/projects/surface.go":    "projects cannot import people (ADR-0054 §3); the predicate must move tier first",
	"internal/modules/signals/resolver.go":    "signals cannot import people (ADR-0054 §3); the predicate must move tier first",
	"internal/modules/signals/warmroom.go":    "signals cannot import people (ADR-0054 §3); the predicate must move tier first",
})

const (
	employmentHelper = "EmploymentIsCurrentSQL"
	primaryHelper    = "CurrentPrimaryEmploymentSQL"
	employmentIssue  = "five statements in three sibling modules are ratified separately: a module may not import people (ADR-0054 §3), so the predicate has to move tier before they can adopt it; see issue 2360"
)

// employmentKind matches a statement that has scoped itself to employments.
//
// `'employment'` ANYWHERE in an IN list, not only first. The pattern used to
// anchor on the opening paren, so `kind IN ('deal_stakeholder', 'employment')`
// was not an employment statement as far as the census was concerned — and a
// hand-written currency test in one would have passed. Ordering inside an IN
// list is the author's whim, which is a poor thing for a gate to depend on.
//
// One level of nesting is allowed inside the list, because `[^)]*` stopped at
// the FIRST close-paren and an item like `(SELECT …)` ended the match before
// the literal. One level and not arbitrary depth: RE2 has no recursion, the
// deeper form does not occur here, and a bounded pattern that says what it
// does is better than an unbounded claim.
var employmentKind = regexp.MustCompile(`kind\s*=\s*'employment'|kind\s+IN\s*\((?:[^()]|\([^()]*\))*'employment'`)

// endedAtCurrency matches a hand-written currency test on ended_at — the bare
// null check that loses a notice period, and the long form that gets the
// semantics right but is still a second copy.
//
// `IS NOT NULL` is matched too. The negation is the same decision made
// backwards, and leaving it out let a statement ask "has this person left?" by
// hand while its sibling half asked "are they still here?" through the helper
// — one query, two definitions, and they disagreed on the day a notice period
// ended.
var endedAtCurrency = regexp.MustCompile(`ended_at\s+IS\s+(NOT\s+)?NULL|ended_at\s*(>|<|>=|<=)`)

// employmentCurrencyOwner is where the definition lives. Its own statements are
// the definition rather than a copy of it.
const employmentCurrencyOwner = "internal/modules/people/employmentcurrency.go"

func TestEveryEmploymentCurrencyTestUsesTheOneDefinition(t *testing.T) {
	// A ratification that stops matching is a ratification for a site that has
	// moved or been fixed, and leaving it in place quietly re-exempts whatever
	// takes its name next.
	defer blockedByTheModuleDAG.AssertAllMatched(t)

	fset := token.NewFileSet()
	var findings []string
	files := handWrittenGoSources(t)
	judged := 0
	for _, path := range files {
		if filepath.ToSlash(path) == employmentCurrencyOwner {
			continue
		}
		// This file holds the planted probes below, which are deliberate
		// defects — judging them would report the gate's own evidence as a
		// finding. Skipped by name rather than by "_test.go", because a real
		// test that hand-writes an employment currency test is still a finding
		// and there is no reason to stop looking at the ones that do not plant
		// anything.
		if filepath.Base(path) == "employmentcurrency_test.go" {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		scope := helperScope{
			qualifier: importAliasOf(file, "github.com/gradionhq/margince/backend/internal/modules/people"),
			inside:    file.Name != nil && file.Name.Name == "people",
		}
		for _, decl := range file.Decls {
			for _, sql := range employmentStatements(decl, scope) {
				judged++
				if !endedAtCurrency.MatchString(sql) {
					continue
				}
				if blockedByTheModuleDAG.Waived(t, filepath.ToSlash(path)) {
					continue
				}
				findings = append(findings, fmt.Sprintf("%s: %s", path, firstEmploymentLine(sql)))
			}
		}
	}
	// A census that judged nothing certifies nothing. The floor is far below the
	// real count so it catches a broken walk, not a changing tree.
	if judged < 10 {
		t.Fatalf("only %d employment statement(s) were judged, so this census covered almost nothing", judged)
	}
	if len(findings) == 0 {
		return
	}
	t.Errorf("these statements decide whether an employment is current by testing ended_at themselves:\n  %s\n\n"+
		"people.%s is the one definition, and it is a DATE comparison: somebody serving three months' "+
		"notice still works there, and reading the column's presence as \"gone\" takes them off their "+
		"employer's contact list the day their notice is filed — with no way back, because ended_at "+
		"cannot be cleared through the API. Call the helper. (%s)",
		strings.Join(findings, "\n  "), employmentHelper, employmentIssue)
}

// employmentStatements returns the SQL statements in a declaration that have
// scoped themselves to employments.
//
// A statement, not a literal. A query that calls the helper is written as
//
//	`… WHERE r.kind = 'employment' AND ` + people.EmploymentIsCurrentSQL("r.ended_at") + ` AND …`
//
// which the parser gives as three separate nodes, so judging each *ast.BasicLit
// on its own splits the question in half: the piece naming the employment kind
// no longer contains the `ended_at` test, and the gate passes over it.
//
// That is not a theoretical gap — it is the shape EVERY site adopted in this
// change now has, so the gate could not have caught a regression at any of
// them. Verified by reintroducing a bare `ended_at IS NULL` as a concatenated
// fragment: the gate reported ok.
//
// A concatenation is therefore flattened first. A call contributes its function
// NAME, which is what makes the helper exemption real rather than dead: the
// helper is called from Go, so its name never appears inside a SQL literal, and
// an exemption looking for it there could never fire.
//
// Per DECLARATION and not per file: a file may hold one query about employments
// and another about deal stakeholders, and asking whether both shapes appear
// somewhere in the same file reports a pairing nobody wrote.
func employmentStatements(decl ast.Decl, people helperScope) []string {
	var out []string
	seen := map[ast.Node]bool{}
	ast.Inspect(decl, func(n ast.Node) bool {
		if seen[n] {
			return false
		}
		text, ok := flattenSQL(n, seen, people)
		if !ok || !employmentKind.MatchString(text) {
			return true
		}
		out = append(out, text)
		return true
	})
	return out
}

// flattenSQL renders a string expression as the text it builds, marking every
// node it consumed so an inner piece is not judged again on its own.
//
// A call to the ONE DEFINITION renders as a neutral marker: the predicate it
// produces exists only at runtime, so the statement's text carries no
// `ended_at` test from it, and a statement that calls the helper is simply a
// statement with nothing hand-written left to find.
//
// That replaced an exemption — "this statement mentions the helper, so skip
// it" — which was too coarse in the direction that matters: a query calling
// the helper for one half and hand-writing the other was skipped WHOLESALE.
// Calling the one definition is not a licence to write a second one beside it.
//
// Any other call renders as its ARGUMENTS and not its name, because a
// formatter holds its SQL in an argument — `fmt.Sprintf(`… kind = 'employment'
// … `, …)` keeps the whole statement inside the call, and a flattener that
// stopped at the callee name would judge nothing. The name is dropped because
// it is not part of the SQL; only the helper's is kept, as the marker that
// says the one definition was reached.
func flattenSQL(n ast.Node, seen map[ast.Node]bool, people helperScope) (string, bool) {
	switch v := n.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return "", false
		}
		seen[n] = true
		return strings.Trim(v.Value, "`\""), true
	case *ast.BinaryExpr:
		if v.Op != token.ADD {
			return "", false
		}
		left, lok := flattenSQL(v.X, seen, people)
		right, rok := flattenSQL(v.Y, seen, people)
		if !lok && !rok {
			return "", false
		}
		seen[n] = true
		return left + right, true
	case *ast.CallExpr:
		seen[n] = true
		if people.isOneDefinition(v) {
			markSeen(v, seen)
			return " " + employmentCalleeName(v) + " ", true
		}
		text := ""
		for _, a := range v.Args {
			if part, ok := flattenSQL(a, seen, people); ok {
				text += part
			}
		}
		return " " + text + " ", true
	case ast.Expr:
		seen[n] = true
		return " ", true
	}
	return "", false
}

// helperScope says how the ONE DEFINITION is reachable from one file, and it is
// a struct rather than a string because the string had two meanings.
//
// `qualifier == ""` was read as "this file IS package people", where a bare
// call is the helper's own. But importAliasOf returns "" for every file that
// does not import people at all — which is most of the tree — so in any of
// them a bare `EmploymentIsCurrentSQL(…)` was accepted as canonical and its
// arguments hidden. `inside` says the one thing the empty string could not.
type helperScope struct {
	qualifier string // the local name people is bound to, "" if not imported
	inside    bool   // this file IS package people
}

// isOneDefinition reports whether the call is people's helper — the PACKAGE as
// well as the name.
//
// The name alone was not enough, and the gap is the one this gate exists to
// close: a helper call's whole subtree is claimed, so `other.
// EmploymentIsCurrentSQL(…)` would have been treated as canonical and its
// arguments hidden, letting a hand-written currency test ride inside a
// lookalike.
func (h helperScope) isOneDefinition(call *ast.CallExpr) bool {
	named := func(n string) bool { return n == employmentHelper || n == primaryHelper }
	switch f := call.Fun.(type) {
	case *ast.Ident:
		return h.inside && named(f.Name)
	case *ast.SelectorExpr:
		pkg, ok := f.X.(*ast.Ident)
		return ok && h.qualifier != "" && pkg.Name == h.qualifier && named(f.Sel.Name)
	}
	return false
}

// importAliasOf returns the local name an import path is bound to in this file,
// or "" if the file does not import it. A dot import returns "" too: a
// dot-imported call is a bare identifier, and the gate would rather miss it
// than name the wrong function.
func importAliasOf(file *ast.File, path string) string {
	for _, spec := range file.Imports {
		if strings.Trim(spec.Path.Value, `"`) != path {
			continue
		}
		if spec.Name != nil {
			if spec.Name.Name == "." {
				return ""
			}
			return spec.Name.Name
		}
		return "people"
	}
	return ""
}

// employmentCalleeName is the function's own name, however it is qualified.
// Prefixed because retentionscope_test.go already has a calleeName in this
// package.
func employmentCalleeName(call *ast.CallExpr) string {
	switch f := call.Fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	}
	return ""
}

// markSeen claims a whole subtree, so nothing inside a helper call is judged as
// though somebody had written it into the statement.
func markSeen(n ast.Node, seen map[ast.Node]bool) {
	ast.Inspect(n, func(c ast.Node) bool {
		if c != nil {
			seen[c] = true
		}
		return true
	})
}

// firstEmploymentLine returns the line of the statement that names the
// employment kind, so the report points at the statement rather than dumping
// it.
func firstEmploymentLine(sql string) string {
	for _, line := range strings.Split(sql, "\n") {
		if employmentKind.MatchString(line) {
			return strings.TrimSpace(line)
		}
	}
	return strings.TrimSpace(strings.Split(sql, "\n")[0])
}

// handWrittenGoSources walks the module for source a person maintains.
func handWrittenGoSources(t *testing.T) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if name := entry.Name(); name == "node_modules" || name == "testdata" || name == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_gen.go") || strings.HasSuffix(name, ".gen.go") {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module for Go source: %v", err)
	}
	if len(paths) < 500 {
		t.Fatalf("the walk found only %d Go files, so this census covered almost nothing", len(paths))
	}
	return paths
}

// employmentProbe is one planted source file and the answer the gate must give
// for it.
type employmentProbe struct {
	name  string
	fires bool
	// mode picks the file the probe is parsed as, because the gate's answers
	// depend on it and a probe that guesses wrong asks a different question
	// than the tree does:
	//
	//   ""         package probe, importing people — an ordinary caller
	//   "people"   package people — the one place a bare call is the helper's
	//   "noimport" package probe, NOT importing people — most of the tree, and
	//              where a bare helper name is somebody else's function
	mode string
	src  string
}

// The census above is a census of ZERO: it passes identically over a clean tree
// and over a detector that has stopped detecting. These read the detector
// directly, which is the half that makes the census mean anything.
//
// Every case here exists because the gate was once green over it.
var employmentProbes = []employmentProbe{
	{"the bare form that shipped, one literal", true, "", `
func read() string {
	return ` + "`" + `SELECT 1 FROM relationship r WHERE r.kind = 'employment' AND r.ended_at IS NULL` + "`" + `
}`},
	{"the same, split across a concatenation", true, "", `
func read() string {
	return ` + "`" + `SELECT 1 FROM relationship r WHERE r.kind = 'employment' AND ` + "`" + ` + ` + "`" + `r.ended_at IS NULL` + "`" + `
}`},
	{"the same, inside a formatter's argument", true, "", `
func read() string {
	return fmt.Sprintf(` + "`" + `SELECT 1 FROM relationship r WHERE r.kind = 'employment' AND r.ended_at IS NULL AND (%s)` + "`" + `, scope)
}`},
	{"the negation, which is the same decision backwards", true, "", `
func read() string {
	return ` + "`" + `SELECT 1 FROM relationship r WHERE r.kind = 'employment' AND r.ended_at IS NOT NULL` + "`" + `
}`},
	{"the helper AND a hand-written test beside it", true, "", `
func read() string {
	return ` + "`" + `SELECT 1 FROM relationship r WHERE r.kind = 'employment' AND ` + "`" + ` + people.EmploymentIsCurrentSQL("r.ended_at") + ` + "`" + ` AND r.ended_at IS NOT NULL` + "`" + `
}`},
	// The name alone is not the helper. markSeen claims a helper call's whole
	// subtree, so a LOOKALIKE would have had its arguments hidden and could
	// have carried a hand-written test through inside them.
	{"a lookalike helper from another package", true, "", `
func read() string {
	return ` + "`" + `SELECT 1 FROM relationship r WHERE r.kind = 'employment' AND ` + "`" + ` + other.EmploymentIsCurrentSQL("r.ended_at IS NULL") + ` + "`" + ` AND 1=1` + "`" + `
}`},

	{"the real helper, qualified", false, "", `
func read() string {
	return ` + "`" + `SELECT 1 FROM relationship r WHERE r.kind = 'employment' AND ` + "`" + ` + people.EmploymentIsCurrentSQL("r.ended_at") + ` + "`" + ` AND r.archived_at IS NULL` + "`" + `
}`},
	{"the real helper, unqualified inside people", false, "people", `
func read() string {
	return ` + "`" + `SELECT 1 FROM relationship r WHERE r.kind = 'employment' AND ` + "`" + ` + EmploymentIsCurrentSQL("r.ended_at") + ` + "`" + ` AND r.archived_at IS NULL` + "`" + `
}`},
	// A bare call outside people names something else entirely.
	{"an unqualified call outside people", true, "", `
func read() string {
	return ` + "`" + `SELECT 1 FROM relationship r WHERE r.kind = 'employment' AND ` + "`" + ` + EmploymentIsCurrentSQL("r.ended_at IS NULL") + ` + "`" + ` AND 1=1` + "`" + `
}`},
	// Another relationship kind is a different question, deliberately not this
	// gate's.
	// `'employment'` need not be FIRST in an IN list. Ordering there is the
	// author's whim, which is a poor thing for a gate to depend on.
	{"an IN list where employment is not first", true, "", `
func read() string {
	return ` + "`" + `SELECT 1 FROM relationship r WHERE r.kind IN ('deal_stakeholder', 'employment') AND r.ended_at IS NULL` + "`" + `
}`},
	// A bare call in a file that simply does not import people names something
	// else. An empty qualifier used to mean both "this file IS people" and
	// "this file does not import people", and most of the tree is the second.
	{"a bare helper name in a file that does not import people", true, "noimport", `
func read() string {
	return ` + "`" + `SELECT 1 FROM relationship r WHERE r.kind = 'employment' AND ` + "`" + ` + EmploymentIsCurrentSQL("r.ended_at IS NULL") + ` + "`" + ` AND 1=1` + "`" + `
}`},
	{"an IN list whose earlier item is a subquery", true, "", `
func read() string {
	return ` + "`" + `SELECT 1 FROM relationship r WHERE r.kind IN ('x', (SELECT k FROM t), 'employment') AND r.ended_at IS NULL` + "`" + `
}`},
	{"a deal_stakeholder edge", false, "", `
func read() string {
	return ` + "`" + `SELECT 1 FROM relationship r WHERE r.kind = 'deal_stakeholder' AND r.ended_at IS NULL` + "`" + `
}`},
	{"an employment query that never asks about currency", false, "", `
func read() string {
	return ` + "`" + `SELECT count(*) FROM relationship r WHERE r.kind = 'employment'` + "`" + `
}`},
}

func TestTheEmploymentDetectorSeesWhatItClaimsTo(t *testing.T) {
	fset := token.NewFileSet()
	for _, tc := range employmentProbes {
		t.Run(tc.name, func(t *testing.T) {
			head := "package probe\n"
			scope := helperScope{qualifier: "people"}
			switch tc.mode {
			case "people":
				head, scope = "package people\n", helperScope{inside: true}
			case "noimport":
				scope = helperScope{}
			default:
				head += "import (\n\t\"fmt\"\n\n\t\"github.com/gradionhq/margince/backend/internal/modules/people\"\n)\n"
			}
			file, err := parser.ParseFile(fset, "probe.go", head+tc.src, 0)
			if err != nil {
				t.Fatalf("the probe does not parse, so it proves nothing: %v", err)
			}
			hit := false
			for _, decl := range file.Decls {
				for _, sql := range employmentStatements(decl, scope) {
					if endedAtCurrency.MatchString(sql) {
						hit = true
					}
				}
			}
			if tc.fires && !hit {
				t.Errorf("the detector missed a hand-written currency test — the census would read green over this:\n%s", tc.src)
			}
			if !tc.fires && hit {
				t.Errorf("the detector reported a statement that asks the one definition, or asks nothing:\n%s", tc.src)
			}
		})
	}
}
