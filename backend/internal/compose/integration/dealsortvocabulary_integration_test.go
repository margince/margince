// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A column the deals list SHOWS is a column the deals list SORTS BY.
//
// The list draws seven columns and its sort vocabulary reached three of them,
// so four headers were dead controls a reader had to learn to ignore — and the
// frontend said so in a comment beside each one rather than being able to fix
// it, because the vocabulary was pinned to a spec set written before the
// columns were. Lars ruled on 2026-08-21 that the rule is the columns, for
// every list in the product.
//
// Two of the four are reachable today and are held here. The other two are
// JOINED columns — a stage orders by its position in its pipeline, a partner by
// the company's name — and the list machinery renders one quoted
// identifier of the row's own table, so they need the sort model to take an
// expression first. That is why this file pins two rather than four, and it is
// stated so a reader does not read the gap as an oversight.

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TestTheDealsListSortsByTheNameColumnItDraws: alphabetical, which a list of
// deals had no way to offer at all.
func TestTheDealsListSortsByTheNameColumnItDraws(t *testing.T) {
	f := setupDealCFV(t)
	// Seeded out of order, so a pass cannot come from insertion order — which
	// is what the default `-created_at` sort would give.
	carla := f.seedScoredDeal(t, "Carla renewal", nil)
	alma := f.seedScoredDeal(t, "Alma expansion", nil)
	brig := f.seedScoredDeal(t, "Brig retainer", nil)

	ascending := "name"
	got, _ := f.listDealIDs(t, deals.ListDealsInput{Sort: &ascending})
	assertIDOrder(t, got, []ids.UUID{alma, brig, carla}, "name ascending")

	descending := "-name"
	got, _ = f.listDealIDs(t, deals.ListDealsInput{Sort: &descending})
	assertIDOrder(t, got, []ids.UUID{carla, brig, alma}, "name descending")
}

// TestTheDealsListSortsByTheStatusColumnItDraws: by the stored value, so the
// three groups sit together.
//
// Alphabetical rather than by lifecycle — lost, open, won — because status is a
// text column and the machinery orders by the column. Asserted as the order it
// actually produces rather than the order a reader might expect, because a test
// that wanted the lifecycle order would be asking for a second copy of the
// vocabulary on the sorting side.
func TestTheDealsListSortsByTheStatusColumnItDraws(t *testing.T) {
	e := Setup(t)
	pipeline, openStage, wonStage := DealFixture(t, e)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, dealCFVPerms)

	openDeal := e.SeedDeal(t, "Still open", pipeline, openStage, &e.Rep1)
	wonDeal := e.SeedDeal(t, "Closed won", pipeline, openStage, &e.Rep1)
	// Through the real transition: a status written by hand would order
	// correctly over a column no writer ever puts that value in.
	if _, err := e.Deals.AdvanceDeal(ctx, ids.From[ids.DealKind](wonDeal), wonInput(wonStage)); err != nil {
		t.Fatalf("winning the deal: %v", err)
	}

	listed := func(spec string) []ids.UUID {
		t.Helper()
		rows, _, err := e.Deals.ListDeals(ctx, deals.ListDealsInput{Sort: &spec})
		if err != nil {
			t.Fatalf("ListDeals(sort=%s): %v", spec, err)
		}
		out := make([]ids.UUID, len(rows))
		for i, d := range rows {
			out[i] = ids.UUID(d.Id)
		}
		return out
	}

	// "open" < "won"
	assertIDOrder(t, listed("status"), []ids.UUID{openDeal, wonDeal}, "status ascending")
	assertIDOrder(t, listed("-status"), []ids.UUID{wonDeal, openDeal}, "status descending")
}

// The refusal still refuses. Widening a vocabulary is the shape most likely to
// widen it too far — a sort that reached any column at all would let a caller
// order by a column the list does not publish, and the refusal is what keeps
// the vocabulary a vocabulary.
func TestTheDealsListStillRefusesAFieldItDoesNotPublish(t *testing.T) {
	f := setupDealCFV(t)
	f.seedScoredDeal(t, "Only deal", nil)

	// A real column of `deal`, deliberately: refusing a nonsense name proves
	// nothing about a vocabulary that might simply be "any column".
	notPublished := "captured_by"
	if _, _, err := f.store.ListDeals(f.ctx, deals.ListDealsInput{Sort: &notPublished}); err == nil {
		t.Fatal("the list sorted by a column it does not publish — the vocabulary has stopped being one, and a caller can now order by anything the table holds")
	}
}
