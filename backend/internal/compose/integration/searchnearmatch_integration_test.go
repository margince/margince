// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A search that finds nothing offers up to five names spelt close to the query.
// They sit behind the same object gate and row scope as a hit.

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func nearIDs(page search.Page) []string {
	out := make([]string, 0, len(page.NearMatches))
	for _, m := range page.NearMatches {
		out = append(out, m.ID.String())
	}
	return out
}

func seedContact(t *testing.T, e *SearchEnv, name string) string {
	t.Helper()
	return e.SeedID(t, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, $2, 'manual', 'human:x')`, name).String()
}

func TestAMissingLetterOffersTheClosestNames(t *testing.T) {
	e := SetupSearch(t)
	anna := seedContact(t, e, "Anna Kowalczyk")

	for _, q := range []string{"anna kowalcyk", "kowalcyk"} {
		page, err := e.Store.Search(e.Admin(), search.Input{Query: q, NearMatches: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Hits) != 0 || len(page.NearMatches) != 1 || page.NearMatches[0].ID.String() != anna {
			t.Errorf("%q: hits %v, near matches %v, want only %s as a near match", q, page.Hits, page.NearMatches, anna)
		}
	}
	page, err := e.Store.Search(e.Admin(), search.Input{Query: "anna kowalczyk", NearMatches: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Hits) != 1 || page.NearMatches != nil {
		t.Errorf("a query that matches answers hits %v and near matches %v, want one hit and none", page.Hits, page.NearMatches)
	}
	page, err = e.Store.Search(e.Admin(), search.Input{Query: "anna kowalcyk"})
	if err != nil {
		t.Fatal(err)
	}
	if page.NearMatches != nil {
		t.Errorf("near matches came back although the caller did not ask: %v", page.NearMatches)
	}
}

func TestNearMatchesCarryTheCallersRowScopeAndSkipArchivedAndOwnCompany(t *testing.T) {
	e := SetupSearch(t)
	e.SeedID(t, `INSERT INTO contact (id, full_name, owner_id, visibility, source, captured_by) VALUES ($1, 'Scoped Kowalczyk', $2, 'owner', 'manual', 'human:x')`, e.Rep3)
	e.SeedID(t, `INSERT INTO contact (id, full_name, source, captured_by, archived_at) VALUES ($1, 'Archived Kowalczyk', 'manual', 'human:x', now())`)
	e.SeedID(t, `INSERT INTO company (id, display_name, is_anchor, source, captured_by) VALUES ($1, 'Kowalczyk Own GmbH', true, 'manual', 'human:x')`)

	rep1, err := e.Store.Search(e.AsTeamRep(e.Rep1, e.Team1), search.Input{Query: "kowalcyk", NearMatches: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep1.NearMatches) != 0 {
		t.Errorf("rep1 is offered %v: a private capture, an archived row or the own company leaked", rep1.NearMatches)
	}
	rep3, err := e.Store.Search(e.AsTeamRep(e.Rep3, e.Team2), search.Input{Query: "kowalcyk", NearMatches: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep3.NearMatches) != 1 || rep3.NearMatches[0].Title != "Scoped Kowalczyk" {
		t.Errorf("the owner is offered %v, want only Scoped Kowalczyk", rep3.NearMatches)
	}
}

func TestNearMatchesRunOnlyOnAPlainEmptyFirstPage(t *testing.T) {
	e := SetupSearch(t)
	for i := range 7 {
		seedContact(t, e, fmt.Sprintf("Kowalczyk%c", 'a'+i))
	}
	company := e.SeedID(t, `INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, 'Nowak Werke', 'manual', 'human:x')`)
	e.SeedID(t, `INSERT INTO lead (id, full_name, company_name, status, source, captured_by) VALUES ($1, 'Ida Fenn', 'Brandtmeier Logistik', 'new', 'manual', 'human:x')`)

	search1 := func(in search.Input) search.Page {
		t.Helper()
		in.NearMatches = true
		page, err := e.Store.Search(e.Admin(), in)
		if err != nil {
			t.Fatal(err)
		}
		return page
	}
	if got := len(search1(search.Input{Query: "kowalcyka"}).NearMatches); got != 5 {
		t.Errorf("seven close names gave %d near matches, want 5", got)
	}
	seedContact(t, e, "Sample Pagemark One")
	seedContact(t, e, "Sample Pagemark Two")
	first, err := e.Store.Search(e.Admin(), search.Input{Query: "pagemark", Limit: 1})
	if err != nil || first.NextCursor == "" {
		t.Fatalf("no cursor to test with: %v %+v", err, first)
	}
	cursor := first.NextCursor
	for what, in := range map[string]search.Input{
		"a cursor":         {Query: "kowalcyka", Cursor: cursor},
		"operator syntax":  {Query: "kowalcyka OR zzz"},
		"a bound":          {Query: "kowalcyka", Within: []ids.UUID{}},
		"another type set": {Query: "kowalcyka", Types: []string{"deal"}},
	} {
		if got := search1(in).NearMatches; len(got) != 0 {
			t.Errorf("%s still offered %v", what, got)
		}
	}
	lead := search1(search.Input{Query: "brandmeier"})
	if len(lead.NearMatches) != 1 || lead.NearMatches[0].Type != "lead" || lead.NearMatches[0].Snippet != "Brandtmeier Logistik" {
		t.Errorf("a lead found by its company name gave %v, want the lead with the company beside it", lead.NearMatches)
	}
	if got := search1(search.Input{Query: "nowk werke"}).NearMatches; len(got) != 1 || got[0].ID != company {
		t.Errorf("a company typo gave %v, want %s", got, company)
	}
}

func TestEveryNearMatchPredicateReadsItsTrigramIndex(t *testing.T) {
	e := SetupSearch(t)
	ctx := context.Background()
	for _, entity := range search.NearNamedBranches() {
		table, expr, _ := search.NearNameExpr(entity, "t")
		tx, err := e.Owner.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
			t.Fatal(err)
		}
		var plan string
		rows, err := tx.Query(ctx, `EXPLAIN SELECT t.id FROM `+table+` t WHERE f_fold_apostrophes(lower($1::text)) <% `+expr, "kowalcyk")
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				t.Fatal(err)
			}
			plan += line + "\n"
		}
		rows.Close()
		if err := tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(plan, "idx_"+entity+"_name_trgm") {
			t.Errorf("the %s near-match predicate does not read idx_%s_name_trgm:\n%s", entity, entity, plan)
		}
	}
}
