// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"maps"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// suggestionsPastTheCensus is more of each suggestion kind than the ranked
// queue reads, so the walk must reach the rest through the scroll tail.
const suggestionsPastTheCensus = decisionCensusForTest + 30

// raisePastTheCensus has the deal scout raise one suggestion on each of count
// new companies, over as many passes as its cap needs.
func (e *scoutEnv) raisePastTheCensus(t *testing.T, count int) {
	t.Helper()
	for i := range count {
		company := e.SeedCompany(t, fmt.Sprintf("Account %d", i+1), &e.Rep1)
		buyer := e.employee(t, fmt.Sprintf("Buyer %d", i+1), company)
		request := e.buyerRequest(t, buyer, e.daysAgo(1), "")
		e.signal(t, "new_opportunity", company, request, e.now, ids.Nil, buyer)
		e.signal(t, "commitment_made", company, request, e.now, ids.Nil)
	}
	raised := 0
	for pass := 0; raised < count; pass++ {
		if pass > count/decisionCensusForTest+1 {
			t.Fatalf("the deal scout raised %d of %d suggestions and stopped", raised, count)
		}
		raised += e.pass(t).Raised
	}
}

// tagPastTheCensus has the tag scout propose one tag on each of count new
// contacts, over as many passes as its cap needs.
func (e *tagScoutEnv) tagPastTheCensus(t *testing.T, count int, now time.Time) {
	t.Helper()
	e.tag(t, "Product X", "product x pricing", true)
	for i := range count {
		contact := e.SeedContact(t, fmt.Sprintf("Tagged %d", i+1), &e.Rep1)
		e.note(e.Admin(), t, "note", "Asked about Product X pricing.", contact, now.Add(-time.Hour))
	}
	raised := 0
	for pass := 0; raised < count; pass++ {
		if pass > count/decisionCensusForTest+1 {
			t.Fatalf("the tag scout raised %d of %d suggestions and stopped", raised, count)
		}
		raised += e.pass(t, now).Raised
	}
}

// suggestionWalker reads every decision source the Decisions view counts,
// suggestion evidence included, so the view can state a total.
func suggestionWalker(e *integration.Env) context.Context {
	perms := integration.AdminWithSignals
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["introduction"] = principal.ObjectGrant{Read: true}
	return e.As(e.AdminUser, nil, perms)
}

// walkSuggestions follows next_cursor through the Decisions view to its end
// and answers how often each row was served, by source, and the first page.
func walkSuggestions(
	ctx context.Context, t *testing.T, e *scoutEnv,
) (map[crmcontracts.WorklistItemSource]map[string]int, crmcontracts.Worklist) {
	t.Helper()
	feed := e.scoutWorklist()
	served := map[crmcontracts.WorklistItemSource]map[string]int{}
	var first crmcontracts.Worklist
	cursor := ""
	for page := 0; ; page++ {
		if page > 40 {
			t.Fatal("the Decisions walk did not end in 40 pages")
		}
		day, err := feed.WorklistView(ctx, "decisions", "all", "", ids.Nil, 25, cursor)
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		if page == 0 {
			first = day
		}
		for _, row := range day.Queue {
			if served[row.Source] == nil {
				served[row.Source] = map[string]int{}
			}
			served[row.Source][row.Id]++
		}
		if day.NextCursor == nil {
			return served, first
		}
		cursor = *day.NextCursor
	}
}

// Past the ranked queue's census depth, the Decisions view continues both
// suggestion kinds through next_cursor until each was served once. The stated
// decisions total is the number of rows the walk served.
func TestTheDecisionsViewPagesThroughEverySuggestionPastTheCensus(t *testing.T) {
	e := setupScout(t)
	e.raisePastTheCensus(t, suggestionsPastTheCensus)
	tags := &tagScoutEnv{Env: e.Env, tags: collections.NewStore(e.DB())}
	tags.tagPastTheCensus(t, suggestionsPastTheCensus, e.now)

	served, first := walkSuggestions(suggestionWalker(e.Env), t, e)

	walked := 0
	for source, rows := range served {
		for id, times := range rows {
			if times != 1 {
				t.Errorf("%s %s was served %d times, want once", source, id, times)
			}
		}
		walked += len(rows)
	}
	for _, source := range []crmcontracts.WorklistItemSource{
		crmcontracts.WorklistItemSourceDealSuggestion, crmcontracts.WorklistItemSourceTagSuggestion,
	} {
		if got := len(served[source]); got != suggestionsPastTheCensus {
			t.Errorf("the walk served %d %s rows, want all %d", got, source, suggestionsPastTheCensus)
		}
	}
	total, stated := totalOf(first, crmcontracts.WorklistCountCategoryDecisions)
	if !stated || total != walked {
		t.Errorf("decisions total = %d (stated %v); the walk served %d", total, stated, walked)
	}
}
