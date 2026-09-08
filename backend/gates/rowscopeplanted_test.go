// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

package gates

// The defect this census is for, planted and run.
//
// #1876 was a read that projected a company id and probed a deal. A gate
// that counted probes rather than matching them to the table would have gone
// green over it — and worse, would then have CERTIFIED it, because the obvious
// way to quiet such a gate is to add a probe over whatever the function already
// had in hand.
//
// So each obstacle is planted here as source and run through the same index the
// tier walk uses. A gate that has never been run against the bug it describes
// has not been shown to work.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// judgePlanted indexes one synthetic file and answers, per reference site,
// whether the enclosing function satisfies the obligation for the table it
// names. The keys are `<function>:<table>`, so a case can assert on the pair
// rather than on a count.
func judgePlanted(t *testing.T, source string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, "planted.go", source, 0)
	if err != nil {
		t.Fatalf("parsing the planted source: %v", err)
	}
	vocab := referenceVocabulary{
		tables:  rowScopedTables(t),
		columns: referenceColumns(t, rowScopedTables(t)),
		edge:    relationshipEndpointTables(t),
	}
	src := tierFile{Path: "planted.go", File: parsed, fset: fset}
	fns := map[string]*rowScopeFnInfo{}
	var sites []referenceSite
	for _, decl := range parsed.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || fn.Body == nil {
			continue
		}
		info := &rowScopeFnInfo{scopes: map[string]bool{}, calls: map[string]bool{}}
		fns[fn.Name.Name] = info
		at := referenceSite{dir: "planted", fn: fn.Name.Name}
		sites = append(sites, indexFuncBody(fn, info, vocab, at, src, map[string]bool{})...)
	}
	judged := map[string]bool{}
	for _, site := range sites {
		judged[site.fn+":"+site.table] = reachesRowScope(fns, site.fn, site.table, map[string]bool{})
	}
	return judged
}

// Obstacle 1: the obligation is per table, so a deal probe does not answer for
// a company the same read hands back.
func TestAProbeOverAnotherTableDoesNotAnswerForTheOneServed(t *testing.T) {
	t.Parallel()
	judged := judgePlanted(t, `package planted

func readDeal(ctx C, tx T, id U) error {
	if err := auth.EnsureVisible(ctx, tx, "deal", id); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, "SELECT d.id, d.company_id FROM deal d WHERE d.id = $1", id)
	_, _ = rows, err
	return nil
}
`)
	satisfied, judgedAtAll := judged["readDeal:company"]
	if !judgedAtAll {
		t.Fatal("the company reference was not judged at all, so nothing below is about the defect")
	}
	if satisfied {
		t.Error("a deal probe answered for a company this read hands back — which is #1876, and " +
			"a gate that accepts it certifies the defect the next author would 'fix' by adding one more probe")
	}
}

// The mirror of the case above, so it is not satisfied by a gate that refuses
// everything: the SAME probe answers for the table it names.
func TestAProbeAnswersForTheTableItNames(t *testing.T) {
	t.Parallel()
	judged := judgePlanted(t, `package planted

func readLink(ctx C, tx T, id U) error {
	if err := auth.EnsureVisible(ctx, tx, "deal", id); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, "SELECT l.id, l.deal_id FROM activity_link l WHERE l.activity_id = $1", id)
	_, _ = rows, err
	return nil
}
`)
	if satisfied, judgedAtAll := judged["readLink:deal"]; !judgedAtAll || !satisfied {
		t.Errorf("a deal reference bounded by a deal probe was reported unscoped (judged=%v satisfied=%v)",
			judgedAtAll, satisfied)
	}
}

// Obstacle 2: an FK named for its ROLE is a reference. `partner_company_id` was one
// of the three #1876 fixed, and the name-derived extractor could not see it.
func TestARoleNamedForeignKeyIsStillAReference(t *testing.T) {
	t.Parallel()
	judged := judgePlanted(t, `package planted

func readPartners(ctx C, tx T) error {
	rows, err := tx.Query(ctx, "SELECT p.id, p.partner_company_id FROM partner p")
	_, _ = rows, err
	return nil
}
`)
	satisfied, judgedAtAll := judged["readPartners:company"]
	if !judgedAtAll {
		t.Fatal("partner_company_id was not read as a company reference — the extractor is back to " +
			"deriving the table from the column's own words, and drops every role-named FK silently")
	}
	if satisfied {
		t.Error("a read that probes nothing reported as scoped")
	}
}

// And the mirror, so the case above is not satisfied by a gate that reports
// everything: the same reference, bounded, passes.
func TestARoleNamedForeignKeyPassesWhenItsOwnTableIsScoped(t *testing.T) {
	t.Parallel()
	judged := judgePlanted(t, `package planted

func readPartners(ctx C, tx T, arg A) error {
	clause, err := auth.ScopeClauseFor(ctx, "company", "o", arg)
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, "SELECT p.id, p.partner_company_id FROM partner p JOIN company o ON o.id = p.partner_company_id WHERE "+clause)
	_, _ = rows, err
	return nil
}
`)
	if satisfied, judgedAtAll := judged["readPartners:company"]; !judgedAtAll || !satisfied {
		t.Errorf("a role-named reference bounded by its own table's scope was reported unscoped "+
			"(judged=%v satisfied=%v) — a census that refuses correct code is one people turn off",
			judgedAtAll, satisfied)
	}
}

// The edge conjunction bounds its endpoints, so a person id projected from a
// relationship that passed it is bounded — reading it as "relationship only"
// would send the next author to add a clause over a column already covered.
func TestTheEdgeConjunctionAnswersForItsEndpoints(t *testing.T) {
	t.Parallel()
	judged := judgePlanted(t, `package planted

func readStakeholders(ctx C, tx T, arg A) error {
	clause, err := auth.EdgeReadScope(ctx, "r", arg)
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, "SELECT r.person_id FROM relationship r WHERE "+clause)
	_, _ = rows, err
	return nil
}
`)
	if satisfied, judgedAtAll := judged["readStakeholders:person"]; !judgedAtAll || !satisfied {
		t.Errorf("a person projected from an edge bounded by the endpoint conjunction was reported "+
			"unscoped (judged=%v satisfied=%v)", judgedAtAll, satisfied)
	}
}

// A count hands back a number. The account-coverage total is deliberately taken
// past the caller's person scope, because the difference between it and the
// visible set is what "contacts you cannot see" means.
func TestACountOfAReferenceIsNotAReference(t *testing.T) {
	t.Parallel()
	judged := judgePlanted(t, `package planted

func countStakeholders(ctx C, tx T) error {
	row := tx.QueryRow(ctx, "SELECT count(DISTINCT r.person_id) FROM relationship r")
	_ = row
	return nil
}
`)
	if _, judgedAtAll := judged["countStakeholders:person"]; judgedAtAll {
		t.Error("a count() over a person id was read as handing one back — the total is a number, and " +
			"scoping it would collapse the difference the coverage card is about")
	}
}

// And the mirror: an aggregate that hands the ids back is still a reference.
func TestAnAggregateThatReturnsTheIdsIsStillAReference(t *testing.T) {
	t.Parallel()
	judged := judgePlanted(t, `package planted

func listStakeholders(ctx C, tx T) error {
	row := tx.QueryRow(ctx, "SELECT array_agg(r.person_id) FROM relationship r")
	_ = row
	return nil
}
`)
	if _, judgedAtAll := judged["listStakeholders:person"]; !judgedAtAll {
		t.Error("array_agg over a person id was dropped as though it were a count — it hands every one " +
			"of those ids back, which is the reference this census is about")
	}
}
