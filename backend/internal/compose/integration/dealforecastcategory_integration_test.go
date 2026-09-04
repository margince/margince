// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The forecast's buckets, as a list a reader can open.
//
// The five forecast tiles partition the pipeline into named buckets and each
// is a number with no door: "which deals are in commit" is the first question
// anyone asks of one, and no wire filter could express it.
//
// Against a real database because the point is the PREDICATE — that an
// uncategorised deal joins no bucket rather than quietly appearing in
// whichever one is asked for, which is a fact about SQL's NULL comparison
// rather than about Go.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/installseam"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type forecastBucketFixture struct {
	store *deals.Store
	ctx   context.Context
}

// named returns the names of the deals one filter selects, so a failure says
// which deal was wrongly in or out rather than only how many.
func (f forecastBucketFixture) named(t *testing.T, category *string) []string {
	t.Helper()
	list, _, err := f.store.ListDeals(f.ctx, deals.ListDealsInput{ForecastCategory: category})
	if err != nil {
		t.Fatalf("ListDeals: %v", err)
	}
	names := make([]string, 0, len(list))
	for _, deal := range list {
		names = append(names, deal.Name)
	}
	return names
}

func TestTheForecastBucketsPartitionTheCategorisedPipeline(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	store := deals.NewStore(e.DB(), installseam.Deals())
	ctx := e.As(e.Rep1, nil, dealCFVPerms)
	f := forecastBucketFixture{store: store, ctx: ctx}

	for name, category := range map[string]string{
		"Commit One":  "commit",
		"Commit Two":  "commit",
		"Best Case":   "best_case",
		"In Pipeline": "pipeline",
		"Omitted":     "omitted",
	} {
		created, err := store.CreateDeal(ctx, deals.CreateDealInput{
			Name: name, PipelineID: pipeline, StageID: open, Source: "ui",
		})
		if err != nil {
			t.Fatalf("seeding %q: %v", name, err)
		}
		bucket := category
		if _, err := store.UpdateDeal(ctx, dealIDOf(ids.UUID(created.Id)),
			deals.UpdateDealInput{ForecastCategory: &bucket}); err != nil {
			t.Fatalf("categorising %q: %v", name, err)
		}
	}
	// The one that belongs to no bucket, which is the case the predicate is
	// really about: it must be absent from every one of the four.
	if _, err := store.CreateDeal(ctx, deals.CreateDealInput{
		Name: "Uncategorised", PipelineID: pipeline, StageID: open, Source: "ui",
	}); err != nil {
		t.Fatalf("seeding the uncategorised deal: %v", err)
	}

	commit := "commit"
	if got := f.named(t, &commit); len(got) != 2 {
		t.Errorf("the commit bucket holds %v, want the two commit deals", got)
	}
	for _, category := range []string{"commit", "best_case", "pipeline", "omitted"} {
		bucket := category
		for _, name := range f.named(t, &bucket) {
			if name == "Uncategorised" {
				t.Errorf("the %q bucket holds the uncategorised deal — a deal in no "+
					"bucket must not join whichever one is asked for", category)
			}
		}
	}
	// Unfiltered still answers everything, so the filter narrows rather than
	// the fixture being thin.
	if got := f.named(t, nil); len(got) != 6 {
		t.Errorf("the unfiltered list holds %d deals, want all 6", len(got))
	}
}
