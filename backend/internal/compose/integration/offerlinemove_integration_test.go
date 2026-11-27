// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A line patch that names a position moves the line there. The lines between
// shift and the offer's lines are numbered 1..n again. The unique position
// index never sees two lines on one number.

import (
	"context"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// threeLineOffer is the baseline offer with a second and third line added.
func threeLineOffer(ctx context.Context, t *testing.T, e *Env) (ids.OfferID, []ids.UUID) {
	t.Helper()
	offerID := baseDraftOffer(ctx, t, e, "Line-move deal")
	for _, description := range []string{"Workshop", "Support"} {
		price, taxRate := int64(5000), "19.00"
		if _, err := e.Deals.AddOfferLineItem(ctx, offerID, deals.OfferLineInputRow{
			Description: &description, Quantity: "1", UnitPriceMinor: &price, TaxRate: &taxRate,
		}); err != nil {
			t.Fatalf("add offer line %q: %v", description, err)
		}
	}
	return offerID, lineOrder(ctx, t, e, offerID)
}

// lineOrder answers the offer's line ids in the order it reads them, and holds
// that their positions run 1..n.
func lineOrder(ctx context.Context, t *testing.T, e *Env, offerID ids.OfferID) []ids.UUID {
	t.Helper()
	offer, err := e.Deals.GetOffer(ctx, offerID, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("read the offer's lines: %v", err)
	}
	var order []ids.UUID
	for i, line := range *offer.LineItems {
		if line.Position != i+1 {
			t.Errorf("line %d holds position %d; want the lines numbered 1..n", i+1, line.Position)
		}
		order = append(order, ids.UUID(line.Id))
	}
	return order
}

func TestMovingALineOntoATakenPositionShiftsTheOthers(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, OfferDeskPerms)
	offerID, lines := threeLineOffer(ctx, t, e)

	top := 1
	if _, err := e.Deals.UpdateOfferLineItem(ctx, offerID, lines[2], deals.UpdateOfferLineInput{Position: &top}); err != nil {
		t.Fatalf("move the last line to the top: %v", err)
	}
	if got, want := lineOrder(ctx, t, e, offerID), []ids.UUID{lines[2], lines[0], lines[1]}; !slices.Equal(got, want) {
		t.Errorf("order after the move = %v, want %v", got, want)
	}

	_, _, evidence := latestOfferUpdateAudit(t, e, offerID)
	assertLineEvidence(t, evidence, lines[2])
	after, ok := evidence["positions_after"].(map[string]any)
	if !ok || len(after) != 3 {
		t.Errorf("evidence = %+v; want positions_after naming the three lines the move renumbered", evidence)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope ->> 'type' = 'offer.updated'
		AND envelope #>> '{entity,id}' = $1::text AND envelope -> 'payload' -> 'changed_fields' ? 'line_items'`,
		offerID.UUID); got < 3 {
		t.Errorf("%d offer.updated events name line_items; want one for each of the two adds and the move", got)
	}

	past := 99
	if _, err := e.Deals.UpdateOfferLineItem(ctx, offerID, lines[2], deals.UpdateOfferLineInput{Position: &past}); err != nil {
		t.Fatalf("move the first line past the end: %v", err)
	}
	if got, want := lineOrder(ctx, t, e, offerID), []ids.UUID{lines[0], lines[1], lines[2]}; !slices.Equal(got, want) {
		t.Errorf("order after moving past the end = %v, want %v", got, want)
	}
}

// A gap left by a removed line closes on the next move. The place counts the
// lines a reader sees, not the numbers they held.
func TestAMoveCountsPlacesAcrossAGap(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, OfferDeskPerms)
	offerID, lines := threeLineOffer(ctx, t, e)
	if _, err := e.Deals.RemoveOfferLineItem(ctx, offerID, lines[1]); err != nil {
		t.Fatalf("remove the middle line: %v", err)
	}

	first := 1
	if _, err := e.Deals.UpdateOfferLineItem(ctx, offerID, lines[2], deals.UpdateOfferLineInput{Position: &first}); err != nil {
		t.Fatalf("move the last line to the top: %v", err)
	}
	if got, want := lineOrder(ctx, t, e, offerID), []ids.UUID{lines[2], lines[0]}; !slices.Equal(got, want) {
		t.Errorf("order after the move = %v, want %v", got, want)
	}
}
