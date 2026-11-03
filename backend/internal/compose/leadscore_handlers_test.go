// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A score made only of fit factors reads no activities at all.
//
// The nil pool is the assertion. Naming opens a transaction only when a
// factor names something, so a fit-only score costs no query. It fixes the
// shape too: a fit factor has no sources to name, and must not claim an empty
// list of them.
func TestAFitOnlyScoreNamesNothingAndReadsNothing(t *testing.T) {
	t.Parallel()
	current := crmcontracts.LeadScoreEntry{Factors: &[]crmcontracts.LeadScoreFactor{
		{Factor: "decision_maker_title", Points: 12},
		{Factor: "high_intent_source", Points: 8},
	}}
	wire := crmcontracts.LeadScoreExplanation{Explained: true, Current: &current}

	got, err := receiptHandlers{}.namedScore(context.Background(), wire)
	if err != nil {
		t.Fatalf("naming a fit-only score: %v", err)
	}
	for _, factor := range *got.Current.Factors {
		if factor.SourceActivities != nil {
			t.Errorf("%s has no sources and was given a list of them", factor.Factor)
		}
	}
}

// A behavioral factor whose id array is empty names an empty LIST.
//
// Absent and empty say different things. A fit factor has no activities. A
// behavioral factor whose sources this reader cannot open has none to show.
// A nil here would reach the wire as `null`, which the contract's array
// rejects.
func TestABehavioralFactorWithNoVisibleSourcesNamesAnEmptyList(t *testing.T) {
	t.Parallel()
	current := crmcontracts.LeadScoreEntry{Factors: &[]crmcontracts.LeadScoreFactor{
		{Factor: "reply", Points: 4, SourceActivityIds: &[]openapi_types.UUID{}},
	}}
	wire := crmcontracts.LeadScoreExplanation{Explained: true, Current: &current}

	got, err := receiptHandlers{}.namedScore(context.Background(), wire)
	if err != nil {
		t.Fatalf("naming a behavioral factor: %v", err)
	}
	named := (*got.Current.Factors)[0].SourceActivities
	if named == nil {
		t.Fatal("an unnamed behavioral factor answered nil, which serializes as null")
	}
	if len(*named) != 0 {
		t.Errorf("a factor with no visible sources named %d activities", len(*named))
	}
}

// Every entry in the history is asked, not only the current one.
//
// The history is where a reader checks what a number was built from last
// week. A naming that reached only the current entry would leave those older
// rows as bare counts.
func TestTheHistoryEntriesAreNamedAsWellAsTheCurrentOne(t *testing.T) {
	t.Parallel()
	current := crmcontracts.LeadScoreEntry{Factors: &[]crmcontracts.LeadScoreFactor{
		{Factor: "reply", Points: 4, SourceActivityIds: &[]openapi_types.UUID{}},
	}}
	history := []crmcontracts.LeadScoreEntry{
		{Factors: &[]crmcontracts.LeadScoreFactor{
			{Factor: "meeting_held", Points: 9, SourceActivityIds: &[]openapi_types.UUID{}},
		}},
		{Factors: &[]crmcontracts.LeadScoreFactor{
			{Factor: "decision_maker_title", Points: 12},
		}},
	}
	wire := crmcontracts.LeadScoreExplanation{Explained: true, Current: &current, History: &history}

	got, err := receiptHandlers{}.namedScore(context.Background(), wire)
	if err != nil {
		t.Fatalf("naming a series: %v", err)
	}
	if (*got.History)[0].Factors == nil {
		t.Fatal("the first history entry lost its factors")
	}
	if named := (*(*got.History)[0].Factors)[0].SourceActivities; named == nil {
		t.Error("a history entry's behavioral factor was not named")
	}
	if named := (*(*got.History)[1].Factors)[0].SourceActivities; named != nil {
		t.Error("a history entry's fit factor was given a list of sources")
	}
}

// One read serves every factor, and each keeps its own ids.
//
// A group per factor would read the activity table once per factor. The
// grouping is what makes that one read. It is asserted on the ids rather
// than on the query count.
func TestEachFactorKeepsItsOwnSourcesAcrossOneRead(t *testing.T) {
	t.Parallel()
	first, second := ids.NewV7(), ids.NewV7()
	groups := [][]ids.UUID{{first}, {second, first}, nil}

	named, err := nameActivityGroups(context.Background(), nil, [][]ids.UUID{nil, nil, nil})
	if err != nil {
		t.Fatalf("naming empty groups: %v", err)
	}
	if len(named) != 3 {
		t.Fatalf("three groups answered %d results", len(named))
	}
	for i, group := range named {
		if group == nil {
			t.Errorf("group %d answered nil, which serializes as null", i)
		}
	}
	if !anyActivityIn(groups) {
		t.Error("a group naming two activities reported nothing to read")
	}
	if anyActivityIn([][]ids.UUID{nil, {}}) {
		t.Error("groups naming nothing reported something to read")
	}
}

// A limit the caller cannot widen.
//
// The page size bounds the read because every entry's sources are filtered
// through the caller's scope, so an unbounded page is unbounded work.
func TestTheHistoryLimitIsBoundedByThePageSize(t *testing.T) {
	t.Parallel()
	ask := func(n int) int {
		return explainInputFrom(crmcontracts.ExplainLeadScoreParams{Limit: &n}).Limit
	}
	if got := ask(10); got != 10 {
		t.Errorf("a limit inside the bounds became %d, want 10", got)
	}
	if got := ask(explainScoreHistoryLimit + 1); got != explainScoreHistoryLimit {
		t.Errorf("a limit past the page size became %d, want the page size %d",
			got, explainScoreHistoryLimit)
	}
	if got := ask(0); got != explainScoreHistoryLimit {
		t.Errorf("a zero limit became %d, want the page size %d", got, explainScoreHistoryLimit)
	}
	if got := ask(-1); got != explainScoreHistoryLimit {
		t.Errorf("a negative limit became %d, want the page size %d", got, explainScoreHistoryLimit)
	}
	if got := explainInputFrom(crmcontracts.ExplainLeadScoreParams{}).Limit; got != explainScoreHistoryLimit {
		t.Errorf("no limit became %d, want the page size %d", got, explainScoreHistoryLimit)
	}
}

// The dials a caller does set are carried, and the ones they leave alone are
// not invented.
func TestTheQueryDialsReachTheRead(t *testing.T) {
	t.Parallel()
	history, cursor := true, "after-this-entry"
	in := explainInputFrom(crmcontracts.ExplainLeadScoreParams{History: &history, Cursor: &cursor})
	if !in.History {
		t.Error("a caller asking for the history did not get it")
	}
	if in.Cursor != cursor {
		t.Errorf("the cursor reached the read as %q, want %q", in.Cursor, cursor)
	}
	if bare := explainInputFrom(crmcontracts.ExplainLeadScoreParams{}); bare.History || bare.Cursor != "" {
		t.Errorf("a bare read asked for history=%v cursor=%q", bare.History, bare.Cursor)
	}
}

// Each group keeps the order its ids arrived in, which is the order the reader
// was asked to recognise. An id the read did not return is left out.
func TestEachGroupKeepsItsOrderAndDropsWhatTheReadDidNotReturn(t *testing.T) {
	t.Parallel()
	first, second, hidden := ids.NewV7(), ids.NewV7(), ids.NewV7()
	named := map[ids.UUID]crmcontracts.ActivityReference{
		first:  {ActivityId: openapi_types.UUID(first), Kind: "email"},
		second: {ActivityId: openapi_types.UUID(second), Kind: "call"},
	}
	// The second group asks for them the other way round, and names the id the
	// read withheld. One activity sits behind two groups.
	got := distributeNamed([][]ids.UUID{{first, second}, {second, hidden, first}, nil}, named)

	if len(got) != 3 {
		t.Fatalf("three groups answered %d results", len(got))
	}
	if len(got[0]) != 2 || got[0][0].Kind != "email" || got[0][1].Kind != "call" {
		t.Errorf("the first group came back as %v, want the order it asked in", got[0])
	}
	if len(got[1]) != 2 || got[1][0].Kind != "call" || got[1][1].Kind != "email" {
		t.Errorf("the second group did not keep its own order: %v", got[1])
	}
	for i, group := range got {
		if group == nil {
			t.Errorf("group %d answered nil, which serializes as null", i)
		}
	}
}
