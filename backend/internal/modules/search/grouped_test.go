// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAGroupedPageKeepsEachTypesBestAndNamesTheTypesThatHadMore(t *testing.T) {
	// The statement returns at most one row past the cap per type.
	hits := []Hit{
		{Type: "activity", ID: ids.NewV7(), Score: 9},
		{Type: "activity", ID: ids.NewV7(), Score: 8},
		{Type: "deal", ID: ids.NewV7(), Score: 4},
		{Type: "activity", ID: ids.NewV7(), Score: 3},
		{Type: "company", ID: ids.NewV7(), Score: 1},
	}
	page := groupedShape{perType: 2}.page(hits)

	kept := map[string]int{}
	for _, hit := range page.Hits {
		kept[hit.Type]++
	}
	if kept["activity"] != 2 || kept["deal"] != 1 || kept["company"] != 1 {
		t.Fatalf("kept %v, want two activities, one deal and one company", kept)
	}
	if slices.ContainsFunc(page.Hits, func(h Hit) bool { return h.Score == 3 }) {
		t.Fatal("the overfetched activity survived; it is the worst of its type and only says there are more")
	}
	if !slices.Equal(page.TypesWithMore, []string{"activity"}) {
		t.Fatalf("TypesWithMore = %v, want [activity]", page.TypesWithMore)
	}
	if page.HasMore || page.NextCursor != "" {
		t.Fatalf("a grouped page is the whole answer, yet it offers a next page: %+v", page)
	}
}

func TestAGroupedPageWithNothingCutNamesNoTypes(t *testing.T) {
	page := groupedShape{perType: 3}.page([]Hit{{Type: "contact", ID: ids.NewV7(), Score: 1}})
	if len(page.Hits) != 1 || page.TypesWithMore == nil || len(page.TypesWithMore) != 0 {
		t.Fatalf("page = %+v, want the one hit and an empty, present list of types with more", page)
	}
}

func TestAGroupedSearchRefusesAPageOrACapItCannotAnswer(t *testing.T) {
	five, zero, tooMany := 5, 0, maxPerType+1
	for name, in := range map[string]Input{
		"a cursor":              {PerType: &five, Cursor: "next"},
		"a limit":               {PerType: &five, Limit: 10},
		"no hits per type":      {PerType: &zero},
		"past the per-type cap": {PerType: &tooMany},
	} {
		_, err := groupedShapeFor(*in.PerType, in)
		var bad *BadQueryError
		if !errors.As(err, &bad) || bad.Field != "per_type" {
			t.Errorf("%s: err = %v, want a 422 naming per_type", name, err)
		}
	}
	if _, err := groupedShapeFor(five, Input{PerType: &five}); err != nil {
		t.Fatalf("a plain grouped request was refused: %v", err)
	}
}

// Every union element at the per-type cap, the employer arm included, can hold
// more rows than the largest ranked page. The page lowers each type's share so
// the answer never costs more than a ranked one, and names the trimmed types.
func TestAGroupedPageFitsInTheLargestRankedPage(t *testing.T) {
	unbounded := math.MaxInt
	largest := storekit.ClampLimit(&unbounded)
	var hits []Hit
	for _, branch := range searchBranches {
		for range maxPerType + 1 {
			hits = append(hits, Hit{Type: branch.entity, ID: ids.NewV7()})
		}
	}
	for range maxPerType + 1 {
		hits = append(hits, Hit{Type: "contact", ID: ids.NewV7()})
	}
	page := groupedShape{perType: maxPerType}.page(hits)
	if len(page.Hits) > largest {
		t.Fatalf("a grouped page of %d hits is past the largest ranked page of %d", len(page.Hits), largest)
	}
	if len(page.Hits) == 0 || len(page.TypesWithMore) != len(searchBranches) {
		t.Errorf("page holds %d hits and says %v has more, want every type present and trimmed", len(page.Hits), page.TypesWithMore)
	}
}

// More types matched than the page holds rows, one row each.
// Lowering the share cannot help, so the cut falls on the types that arrive last.
func TestAGroupedPageWithMoreTypesThanRowsStaysWithinTheRankedPageLimit(t *testing.T) {
	limit := rankedPageLimit()
	var hits []Hit
	for i := range limit + 50 {
		hits = append(hits, Hit{Type: fmt.Sprintf("ext:unit:kind%03d", i), ID: ids.NewV7()})
	}
	page := groupedShape{perType: maxPerType}.page(hits)
	if len(page.Hits) != limit {
		t.Fatalf("a grouped page of %d types holds %d hits, want the limit %d", len(hits), len(page.Hits), limit)
	}
	if page.Hits[limit-1].ID != hits[limit-1].ID {
		t.Errorf("the page kept hits out of order: its last hit is not the %dth that arrived", limit)
	}
}

func TestAGroupedPageNamesTheTypesThatRanOutOfTimeInBranchOrder(t *testing.T) {
	t.Parallel()
	hits := []Hit{{Type: "contact", ID: ids.NewV7()}, {Type: "contact", ID: ids.NewV7()}}
	page := groupedShape{perType: 1}.pageWith(hits, []string{"activity", "company"})
	if want := []string{"company", "activity"}; !slices.Equal(page.TypesTimedOut, want) {
		t.Errorf("TypesTimedOut = %v, want %v, the order of searchBranches", page.TypesTimedOut, want)
	}
	if slices.Contains(page.TypesWithMore, "activity") || !slices.Equal(page.TypesWithMore, []string{"contact"}) {
		t.Errorf("TypesWithMore = %v, want [contact] and no type that timed out", page.TypesWithMore)
	}
	if whole := (groupedShape{perType: 1}).page(nil); whole.TypesTimedOut == nil || len(whole.TypesTimedOut) != 0 {
		t.Errorf("a grouped page where every type answered has TypesTimedOut = %#v, want non-nil and empty", whole.TypesTimedOut)
	}
}

func TestTheLaneWithARankSampleRunsLast(t *testing.T) {
	t.Parallel()
	got := laneOrder([]string{"activity", "contact", "company", "lead"})
	if want := []string{"contact", "company", "lead", "activity"}; !slices.Equal(got, want) {
		t.Errorf("laneOrder = %v, want %v", got, want)
	}
}
