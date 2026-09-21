// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Which closing a deal is on, across the moves that change it.
//
// This is the identity everything written about an outcome hangs from, so what
// matters is not that it exists but that it MOVES when the deal closes again
// and stays put when nothing closed. A loss in March and a win in June are two
// outcomes, and anything attached to the first must not silently reattach
// itself to the second.

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAClosingOccurrenceNamesTheMoveThatClosedTheDeal(t *testing.T) {
	e := Setup(t)
	pipeline, open, won := DealFixture(t, e)
	admin := e.Admin()
	deal := e.SeedDeal(t, "Closed once", pipeline, open, &e.Rep1)
	id := ids.From[ids.DealKind](deal)

	// An open deal is on no closing at all. Not an empty string, not a
	// placeholder: there is nothing to have an opinion about yet.
	before, err := e.Deals.GetDeal(admin, id, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if before.ClosingOccurrenceId != nil {
		t.Fatalf("an open deal reports closing %s, want none", before.ClosingOccurrenceId)
	}

	closed, err := e.Deals.AdvanceDeal(admin, id, deals.AdvanceDealInput{
		ToStageID: won, WonWithoutContractReason: WonByImport(),
	})
	if err != nil {
		t.Fatalf("closing the deal: %v", err)
	}
	if closed.ClosingOccurrenceId == nil {
		t.Fatal("a closed deal reports no closing, so nothing can be written about its outcome")
	}

	// It names a REAL history row, on this deal, which is what the composite
	// foreign key on a review will check.
	if got := e.WsCount(t,
		`SELECT count(*) FROM deal_stage_history WHERE id = $1 AND deal_id = $2`,
		*closed.ClosingOccurrenceId, deal); got != 1 {
		t.Fatal("the reported closing is not a stage-history row on this deal")
	}
}

// The case the whole design exists for.
func TestReopeningAndReclosingIsADifferentClosing(t *testing.T) {
	e := Setup(t)
	pipeline, open, won := DealFixture(t, e)
	admin := e.Admin()
	deal := e.SeedDeal(t, "Closed twice", pipeline, open, &e.Rep1)
	id := ids.From[ids.DealKind](deal)

	first, err := e.Deals.AdvanceDeal(admin, id, deals.AdvanceDealInput{
		ToStageID: won, WonWithoutContractReason: WonByImport(),
	})
	if err != nil {
		t.Fatalf("first close: %v", err)
	}
	firstClosing := *first.ClosingOccurrenceId

	// Reopened: the deal is on no closing. The old one is still in history and
	// still readable — it simply is not the one this deal is on.
	reopened, err := e.Deals.AdvanceDeal(admin, id, deals.AdvanceDealInput{ToStageID: open})
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	if reopened.ClosingOccurrenceId != nil {
		t.Fatalf("a reopened deal still reports closing %s", reopened.ClosingOccurrenceId)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM deal_stage_history WHERE id = $1`,
		firstClosing); got != 1 {
		t.Fatal("reopening deleted the first closing from history")
	}

	// Closed again: a NEW occurrence. A review of the first close must not
	// silently become a review of this one.
	second, err := e.Deals.AdvanceDeal(admin, id, deals.AdvanceDealInput{
		ToStageID: won, WonWithoutContractReason: WonByImport(),
	})
	if err != nil {
		t.Fatalf("second close: %v", err)
	}
	if second.ClosingOccurrenceId == nil {
		t.Fatal("the re-closed deal reports no closing")
	}
	if *second.ClosingOccurrenceId == firstClosing {
		t.Fatal("the second close reports the first closing, so the two outcomes are indistinguishable")
	}
}

// An unrelated edit does not move the closing. Without this the test above
// could be passing because the occurrence changes on any write at all, which
// would make every review stale the moment somebody fixed a typo.
func TestAnUnrelatedEditLeavesTheClosingWhereItIs(t *testing.T) {
	e := Setup(t)
	pipeline, open, won := DealFixture(t, e)
	admin := e.Admin()
	deal := e.SeedDeal(t, "Closed and edited", pipeline, open, &e.Rep1)
	id := ids.From[ids.DealKind](deal)

	closed, err := e.Deals.AdvanceDeal(admin, id, deals.AdvanceDealInput{
		ToStageID: won, WonWithoutContractReason: WonByImport(),
	})
	if err != nil {
		t.Fatal(err)
	}

	renamed := "Closed and edited, renamed"
	after, err := e.Deals.UpdateDeal(admin, id, deals.UpdateDealInput{Name: &renamed})
	if err != nil {
		t.Fatalf("renaming a closed deal: %v", err)
	}
	if after.ClosingOccurrenceId == nil || *after.ClosingOccurrenceId != *closed.ClosingOccurrenceId {
		t.Fatalf("a rename moved the closing from %s to %v",
			*closed.ClosingOccurrenceId, after.ClosingOccurrenceId)
	}
}
