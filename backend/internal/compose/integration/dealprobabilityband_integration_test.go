// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The deal list's probability dials read the probability the forecast weights
// by: a deal's own where set, else its stage's. Against a real database
// because the point is the SQL helper both read.

import (
	"context"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/installseam"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type probabilityFixture struct {
	store *deals.Store
	ctx   context.Context
}

// named returns the names of the deals one probability range selects, so a
// failure says which deal was wrongly in or out.
func (f probabilityFixture) named(t *testing.T, low, high *int) []string {
	t.Helper()
	in := deals.ListDealsInput{}
	in.WinProbabilityMin, in.WinProbabilityMax = low, high
	list, _, err := f.store.ListDeals(f.ctx, in)
	if err != nil {
		t.Fatalf("ListDeals: %v", err)
	}
	names := make([]string, 0, len(list))
	for _, deal := range list {
		names = append(names, deal.Name)
	}
	slices.Sort(names)
	return names
}

func TestTheProbabilityBandsPartitionTheOpenPipeline(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	store := deals.NewStore(e.DB(), installseam.Deals())
	ctx := e.As(e.Rep1, nil, dealCFVPerms)
	f := probabilityFixture{store: store, ctx: ctx}

	var stage int
	for name, override := range map[string]*int{"Set high": new(95), "Set low": new(5), "Follows stage": nil} {
		created, err := store.CreateDeal(ctx, deals.CreateDealInput{
			Name: name, PipelineID: pipeline, StageID: open, Source: "manual",
		})
		if err != nil {
			t.Fatalf("seeding %q: %v", name, err)
		}
		stage = *created.StageWinProbability
		if override == nil {
			continue
		}
		updated, err := store.UpdateDeal(ctx, dealIDOf(ids.UUID(created.Id)), deals.UpdateDealInput{WinProbability: override})
		if err != nil {
			t.Fatalf("setting %q's probability: %v", name, err)
		}
		if *updated.WinProbability != *override || string(*updated.WinProbabilitySource) != "human" {
			t.Errorf("%q reads %d%% from %s, want %d%% set by hand", name,
				*updated.WinProbability, *updated.WinProbabilitySource, *override)
		}
	}
	if stage < 10 || stage > 90 {
		t.Fatalf("the fixture's open stage is at %d%%, which the bands below cannot tell apart", stage)
	}

	if got := f.named(t, new(90), nil); !slices.Equal(got, []string{"Set high"}) {
		t.Errorf("90%% or more holds %v, want the deal set to 95%%", got)
	}
	if got := f.named(t, nil, new(9)); !slices.Equal(got, []string{"Set low"}) {
		t.Errorf("under 10%% holds %v, want the deal set to 5%%", got)
	}
	if got := f.named(t, new(stage), new(stage)); !slices.Equal(got, []string{"Follows stage"}) {
		t.Errorf("exactly the stage's %d%% holds %v, want the deal that follows its stage", stage, got)
	}
}
