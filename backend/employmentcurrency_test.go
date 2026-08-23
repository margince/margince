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
// reach it directly; three sibling modules cannot — four statements across
// activities, projects and signals — and the predicate would have to move tier
// before they could. That is an architecture decision with an
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
	employmentIssue  = "four statements in three sibling modules are ratified separately: a module may not import people (ADR-0054 §3), so the predicate has to move tier before they can adopt it; see issue 2360"
)

// employmentKind matches a statement that has scoped itself to employments.
// Either spelling counts, because both appear: the column compared to the
// literal, and the same test written into a join condition.
var employmentKind = regexp.MustCompile(`kind\s*(=|IN)\s*\(?\s*'employment'`)

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
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			for _, sql := range employmentStatements(decl) {
				judged++
				if !endedAtCurrency.MatchString(sql) {
					continue
				}
				if strings.Contains(sql, employmentHelper) || strings.Contains(sql, primaryHelper) {
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

// employmentStatements returns the SQL literals in a declaration that have
// scoped themselves to employments.
//
// Per DECLARATION and not per file: a file may hold one query about
// employments and another about deal stakeholders, and asking whether both
// shapes appear somewhere in the same file reports a pairing nobody wrote.
func employmentStatements(decl ast.Decl) []string {
	var out []string
	ast.Inspect(decl, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		text := lit.Value
		if !employmentKind.MatchString(text) {
			return true
		}
		out = append(out, text)
		return true
	})
	return out
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
