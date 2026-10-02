// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"errors"
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

// Every union element at the per-type cap — each branch and the employer arm —
// must still fit the largest ranked page, or a grouped request becomes the
// cheapest way to ask for more than a ranked one is allowed. A branch added to
// the table counts here on its own.
func TestAGroupedPageFitsInTheLargestRankedPage(t *testing.T) {
	unbounded := math.MaxInt
	elements := len(searchBranches) + 1
	if largest := storekit.ClampLimit(&unbounded); elements*maxPerType > largest {
		t.Fatalf("%d union elements at %d per type is %d hits, past the largest ranked page of %d",
			elements, maxPerType, elements*maxPerType, largest)
	}
}
