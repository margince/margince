// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The batched deal-figures read, against a real database.
//
// Its whole claim is that it answers under the reader's own row scope, and that
// is SQL: a unit test with hand-built rows cannot fail it. The admission is
// asserted as hard as the refusal, because a read that answered nobody would
// pass a refusal-only suite while leaving every card blank.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedFiguresDeal writes one deal owned by the given rep, with the figures a
// card states.
func seedFiguresDeal(t *testing.T, owner ids.UUID, amount int64) ids.UUID {
	t.Helper()
	conn := OwnerConn(t)
	pipeline := SeedIDRow(t, conn, `INSERT INTO pipeline (id, name, is_default, position)
		VALUES ($1, 'Sales', true, 0)`)
	stage := SeedIDRow(t, conn, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		VALUES ($1, $2, 'Qualify', 0, 'open', 10)`, pipeline)
	return SeedIDRow(t, conn, `INSERT INTO deal
		(id, owner_id, name, pipeline_id, stage_id, amount_minor, currency, expected_close_date,
		 source, captured_by)
		VALUES ($1, $2, 'Northstar renewal', $3, $4, $5, 'EUR', DATE '2026-08-28', 'manual', 'human:x')`,
		owner, pipeline, stage, amount)
}

// A deal the reader may see comes back with the figures a card states.
func TestDealFiguresAnswerADealTheReaderMaySee(t *testing.T) {
	e := Setup(t)
	const amount = int64(160_100_00)
	dealID := seedFiguresDeal(t, e.Rep1, amount)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)

	figures, err := e.Deals.Figures(rep, []ids.UUID{dealID})
	if err != nil {
		t.Fatalf("reading the deal's figures: %v", err)
	}

	got, ok := figures[dealID]
	if !ok {
		t.Fatal("a deal the reader owns did not come back at all")
	}
	if got.AmountMinor == nil || *got.AmountMinor != amount {
		t.Fatalf("the deal came back worth %v, wanted %d", got.AmountMinor, amount)
	}
	if got.Currency != "EUR" {
		t.Fatalf("the deal came back in %q — a figure whose units are unnamed is dropped", got.Currency)
	}
	if got.ExpectedCloseDate == nil {
		t.Fatal("the deal came back with no close date, which is half of why a card is urgent")
	}
	if got.OwnerID != e.Rep1 {
		t.Fatalf("the deal came back owned by %v, wanted %v", got.OwnerID, e.Rep1)
	}
}

// An archived deal is not answered. A card naming a deal nobody works any more
// would send a rep at a closed conversation.
func TestDealFiguresWithholdAnArchivedDeal(t *testing.T) {
	e := Setup(t)
	dealID := seedFiguresDeal(t, e.Rep1, 50_000_00)
	if _, err := OwnerConn(t).Exec(context.Background(),
		`UPDATE deal SET archived_at = now() WHERE id = $1`, dealID); err != nil {
		t.Fatalf("archiving the deal: %v", err)
	}
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)

	figures, err := e.Deals.Figures(rep, []ids.UUID{dealID})
	if err != nil {
		t.Fatalf("an archived deal failed the read: %v", err)
	}

	if _, found := figures[dealID]; found {
		t.Fatal("an archived deal came back")
	}
}

// Asking about nothing answers nothing. A page whose rows all carry their own
// figures must not pay for a read.
func TestDealFiguresAnswerNothingForNoIDs(t *testing.T) {
	e := Setup(t)

	figures, err := e.Deals.Figures(e.Admin(), nil)
	if err != nil {
		t.Fatalf("asking about no deals: %v", err)
	}
	if len(figures) != 0 {
		t.Fatalf("asking about no deals answered %d", len(figures))
	}
}
