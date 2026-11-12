// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A declared kind is a searchable type. It is found by the words of its title.
// Only a caller who may read the kind sees a hit, and an archived row is never one.
// A hit can be dropped by id like any other.

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const historyDemoKind = "ext:historydemo:visit"

func searchTitles(t *testing.T, e *integration.Env, perms principal.Permissions, in search.Input) []string {
	t.Helper()
	page, err := search.NewStore(e.DB()).Search(e.As(e.Rep1, nil, perms), in)
	if err != nil {
		t.Fatalf("search %+v: %v", in, err)
	}
	var out []string
	for _, hit := range page.Hits {
		out = append(out, hit.Type+":"+hit.Title)
	}
	return out
}

func TestAnExtensionKindIsSearchableByItsTitleForACallerWhoMayReadIt(t *testing.T) {
	e := integration.Setup(t)
	registerHistoryVisits(t)
	e.WsExec(t, `INSERT INTO ext.`+historyDemoTable+` (code, site, archived_at) VALUES
		('v1', 'Berlin site', NULL), ('v2', 'Berlin annex', NULL), ('v3', 'Berlin old', now()), ('v4', 'Munich site', NULL)`)
	reader := principal.Permissions{
		RowScope: principal.RowScopeAll,
		Objects:  map[string]principal.ObjectGrant{"ext_historydemo_visit": {Read: true}},
	}

	got := searchTitles(t, e, reader, search.Input{Query: "berlin", Types: []string{historyDemoKind}})
	if len(got) != 2 {
		t.Fatalf("hits = %v, want the two live Berlin visits and neither the archived one nor Munich", got)
	}
	for _, hit := range got {
		if hit[:len(historyDemoKind)] != historyDemoKind {
			t.Errorf("hit %q is not typed %s", hit, historyDemoKind)
		}
	}

	denied := principal.Permissions{RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{"contact": {Read: true}}}
	if got := searchTitles(t, e, denied, search.Input{Query: "berlin", Types: []string{historyDemoKind}}); len(got) != 0 {
		t.Errorf("a caller without the kind's read right was served %v", got)
	}
}

func TestAnExtensionKindIsInTheDefaultTypesOnlyWhenAskedFor(t *testing.T) {
	e := integration.Setup(t)
	registerHistoryVisits(t)
	e.WsExec(t, `INSERT INTO ext.`+historyDemoTable+` (code, site) VALUES ('v1', 'Berlin site')`)
	e.SeedContact(t, "Berlin Contact", nil)
	perms := principal.Permissions{
		RowScope: principal.RowScopeAll,
		Objects:  map[string]principal.ObjectGrant{"contact": {Read: true}, "ext_historydemo_visit": {Read: true}},
	}
	if got := searchTitles(t, e, perms, search.Input{Query: "berlin"}); len(got) != 1 {
		t.Errorf("a search naming no types returned %v; extension kinds join only when asked for", got)
	}
	if got := searchTitles(t, e, perms, search.Input{Query: "berlin", WithExtensions: true}); len(got) != 2 {
		t.Errorf("a search asking for extension kinds returned %v, want the contact and the visit", got)
	}
}

func TestExcludedIdsAreDroppedBeforeRankingAndCounting(t *testing.T) {
	e := integration.Setup(t)
	keep := e.SeedContact(t, "Ines Vogel", nil)
	drop := e.SeedContact(t, "Ines Vogt", nil)
	perms := principal.Permissions{RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{"contact": {Read: true}}}
	perType := 1
	page, err := search.NewStore(e.DB()).Search(e.As(e.Rep1, nil, perms), search.Input{
		Query: "ines", Types: []string{"contact"}, PerType: &perType, Exclude: []ids.UUID{drop},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Hits) != 1 || page.Hits[0].ID != keep {
		t.Errorf("hits = %+v, want only the contact that was not excluded", page.Hits)
	}
	if len(page.TypesWithMore) != 0 {
		t.Errorf("types_with_more = %v; the excluded contact must not count as more", page.TypesWithMore)
	}
}
