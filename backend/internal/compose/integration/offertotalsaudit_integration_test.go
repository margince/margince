// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What a line write records about the OFFER (E03.17/E03.21a). Adding a line,
// editing one, accepting a staged one, removing one and staging a batch all
// audit an update to the offer, and the only field transition any of them
// makes on that record is the three stored money columns: an audit row that
// cannot say what they held before renders half a change in field history and
// leaves nothing to restore the numbers from.
// Which line was touched, and what that line's own fields held, is context
// ABOUT the write, not a field the offer has, so it belongs in evidence — an
// image carrying it puts a change on the screen that never happened to the
// record.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/modules/deals"
	"github.com/gradionhq/margince/backend/internal/platform/database"
	"github.com/gradionhq/margince/backend/internal/platform/database/storekit"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// offerMoneyImage is one side of an offer update's audit images: the three
// money columns, plus every line-context key a line write has to keep OUT of
// one. A non-nil line-context field here IS the defect.
type offerMoneyImage struct {
	NetMinor         *int64  `json:"net_minor"`
	TaxMinor         *int64  `json:"tax_minor"`
	GrossMinor       *int64  `json:"gross_minor"`
	LineAdded        *bool   `json:"line_added"`
	StagedLinesAdded *int    `json:"staged_lines_added"`
	LineID           *string `json:"line_id"`
	LineRemoved      *bool   `json:"line_removed"`
	ProposalState    *string `json:"proposal_state"`
	Description      *string `json:"description"`
	UnitPriceMinor   *int64  `json:"unit_price_minor"`
}

// latestOfferUpdateAudit reads the newest update row the offer's own writes
// left behind. Ordered by id, which is a v7 and therefore monotonic within
// the transaction that minted it — occurred_at alone ties.
func latestOfferUpdateAudit(t *testing.T, e *Env, offerID ids.OfferID) (before, after *offerMoneyImage, evidence map[string]any) {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT before, after, evidence FROM audit_log
			 WHERE entity_type = 'offer' AND entity_id = $1 AND action = 'update'
			 ORDER BY id DESC LIMIT 1`, offerID.UUID).Scan(&before, &after, &evidence)
	}); err != nil {
		t.Fatalf("reading the offer's update audit row: %v", err)
	}
	return before, after, evidence
}

func assertOfferMoneyImage(t *testing.T, img *offerMoneyImage, side string, net, tax, gross int64) {
	t.Helper()
	if img == nil {
		t.Fatalf("the audit row carries no %s image at all: the offer's money moved and nothing records what from", side)
	}
	if img.NetMinor == nil || img.TaxMinor == nil || img.GrossMinor == nil {
		t.Fatalf("the %s image is missing one of net_minor/tax_minor/gross_minor: "+
			"the offer's own field transition went unrecorded", side)
	}
	if *img.NetMinor != net || *img.TaxMinor != tax || *img.GrossMinor != gross {
		t.Errorf("%s image = net %d, tax %d, gross %d; want %d, %d, %d",
			side, *img.NetMinor, *img.TaxMinor, *img.GrossMinor, net, tax, gross)
	}
	for key, present := range map[string]bool{
		"line_added":         img.LineAdded != nil,
		"staged_lines_added": img.StagedLinesAdded != nil,
		"line_id":            img.LineID != nil,
		"line_removed":       img.LineRemoved != nil,
		"proposal_state":     img.ProposalState != nil,
		"description":        img.Description != nil,
		"unit_price_minor":   img.UnitPriceMinor != nil,
	} {
		if present {
			t.Errorf("the %s image carries %s; that context is ABOUT the mutation and belongs in evidence, "+
				"where field history cannot read it as a field on the offer", side, key)
		}
	}
}

// Adding a line moves the offer's money. The audit row says what it moved
// FROM, and says separately — in evidence — that a line was what moved it.
func TestAddOfferLineItemAuditsTheTotalsTheLineMoved(t *testing.T) {
	e := Setup(t)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, offerDeskPerms)
	offerID := baseDraftOffer(ctx, t, e, "Line-add audit deal")

	// A second line: 1 × 50.00 @19% → net 5000, tax 950, on top of the
	// baseline's 10000 / 1900 / 11900.
	description, price, taxRate := "Workshop", int64(5000), "19.00"
	if _, err := e.Deals.AddOfferLineItem(ctx, offerID, deals.OfferLineInputRow{
		Description: &description, Quantity: "1", UnitPriceMinor: &price, TaxRate: &taxRate,
	}); err != nil {
		t.Fatalf("add offer line: %v", err)
	}

	before, after, evidence := latestOfferUpdateAudit(t, e, offerID)
	assertOfferMoneyImage(t, before, "before", 10000, 1900, 11900)
	assertOfferMoneyImage(t, after, "after", 15000, 2850, 17850)
	if added, ok := evidence["line_added"].(bool); !ok || !added {
		t.Errorf("evidence = %+v; want line_added true — the fact that a line was added is what evidence is for", evidence)
	}
}

// Staging an AI batch changes nothing a buyer can see, and the audit row is
// where that claim is written down: before and after hold the same three
// numbers, which is only readable because both images are recorded.
func TestAddStagedOfferLinesAuditsTheTotalsItLeftStanding(t *testing.T) {
	e := Setup(t)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, offerDeskPerms)
	offerID := baseDraftOffer(ctx, t, e, "Staged-audit deal")

	if _, err := e.Deals.AddStagedOfferLines(agentOfferDraftCtx(e), offerID, groundedStagedLines()); err != nil {
		t.Fatalf("add staged offer lines: %v", err)
	}

	before, after, evidence := latestOfferUpdateAudit(t, e, offerID)
	assertOfferMoneyImage(t, before, "before", 10000, 1900, 11900)
	assertOfferMoneyImage(t, after, "after", 10000, 1900, 11900)
	staged, ok := evidence["staged_lines_added"].(float64)
	if !ok || int(staged) != len(groundedStagedLines()) {
		t.Errorf("evidence = %+v; want staged_lines_added %d", evidence, len(groundedStagedLines()))
	}
}

// soleLineID answers the id of the baseline offer's only line — the line the
// edit and removal cases below act on.
func soleLineID(ctx context.Context, t *testing.T, e *Env, offerID ids.OfferID) ids.UUID {
	t.Helper()
	offer, err := e.Deals.GetOffer(ctx, offerID, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("read the offer's lines: %v", err)
	}
	if offer.LineItems == nil || len(*offer.LineItems) != 1 {
		t.Fatalf("the seeded offer does not carry exactly one line: %+v", offer.LineItems)
	}
	return ids.UUID((*offer.LineItems)[0].Id)
}

// assertLineEvidence holds the other half of the split: the line context the
// images must not carry has to be somewhere, and evidence is where.
func assertLineEvidence(t *testing.T, evidence map[string]any, lineID ids.UUID) {
	t.Helper()
	if got, _ := evidence["line_id"].(string); got != lineID.String() {
		t.Errorf("evidence = %+v; want line_id %s — which line moved the offer is what evidence is for", evidence, lineID)
	}
}

// lineEvidenceSide reads one side of the line's own field patch out of
// evidence, where a line write puts it.
func lineEvidenceSide(t *testing.T, evidence map[string]any, side string) map[string]any {
	t.Helper()
	patch, ok := evidence[side].(map[string]any)
	if !ok {
		t.Fatalf("evidence = %+v; want %s to carry the line's own fields", evidence, side)
	}
	return patch
}

// Editing a line re-derives the offer's money. The audit row says what the
// offer's totals moved from, and the line's own field patch — which is not a
// field of the offer — is in evidence.
func TestUpdateOfferLineItemAuditsTheTotalsTheEditMoved(t *testing.T) {
	e := Setup(t)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, offerDeskPerms)
	offerID := baseDraftOffer(ctx, t, e, "Line-edit audit deal")
	lineID := soleLineID(ctx, t, e, offerID)

	// The baseline line doubles in price: 1 × 200.00 @19% → 20000 / 3800.
	raised := int64(20000)
	if _, err := e.Deals.UpdateOfferLineItem(ctx, offerID, lineID, deals.UpdateOfferLineInput{UnitPriceMinor: &raised}); err != nil {
		t.Fatalf("update offer line: %v", err)
	}

	before, after, evidence := latestOfferUpdateAudit(t, e, offerID)
	assertOfferMoneyImage(t, before, "before", 10000, 1900, 11900)
	assertOfferMoneyImage(t, after, "after", 20000, 3800, 23800)
	assertLineEvidence(t, evidence, lineID)
	if priced, _ := lineEvidenceSide(t, evidence, "line_before")["unit_price_minor"].(float64); int64(priced) != 10000 {
		t.Errorf("evidence line_before = %+v; want the line's prior unit_price_minor 10000", evidence["line_before"])
	}
	if priced, _ := lineEvidenceSide(t, evidence, "line_after")["unit_price_minor"].(float64); int64(priced) != raised {
		t.Errorf("evidence line_after = %+v; want the line's new unit_price_minor %d", evidence["line_after"], raised)
	}
}

// An edit that touches no money still records both money images. Equality is
// a claim — "renaming a line moved nothing a buyer pays" is only readable
// from a recorded pair, and field history emits nothing for a key that is
// equal on both sides, so the row costs the screen nothing.
func TestUpdateOfferLineItemAuditsTheTotalsARenameLeftStanding(t *testing.T) {
	e := Setup(t)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, offerDeskPerms)
	offerID := baseDraftOffer(ctx, t, e, "Line-rename audit deal")
	lineID := soleLineID(ctx, t, e, offerID)

	renamed := "Retainer (annual)"
	if _, err := e.Deals.UpdateOfferLineItem(ctx, offerID, lineID, deals.UpdateOfferLineInput{Description: &renamed}); err != nil {
		t.Fatalf("rename offer line: %v", err)
	}

	before, after, evidence := latestOfferUpdateAudit(t, e, offerID)
	assertOfferMoneyImage(t, before, "before", 10000, 1900, 11900)
	assertOfferMoneyImage(t, after, "after", 10000, 1900, 11900)
	assertLineEvidence(t, evidence, lineID)
	if got, _ := lineEvidenceSide(t, evidence, "line_after")["description"].(string); got != renamed {
		t.Errorf("evidence line_after = %+v; want the line's new description %q", evidence["line_after"], renamed)
	}
}

// Accepting a staged line is what makes it count, so the offer's money moves
// on acceptance; the state the line came from is line context, not an offer
// field.
func TestAcceptOfferLineItemAuditsTheTotalsItBrought(t *testing.T) {
	e := Setup(t)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, offerDeskPerms)
	offerID := baseDraftOffer(ctx, t, e, "Line-accept audit deal")

	staged, err := e.Deals.AddStagedOfferLines(agentOfferDraftCtx(e), offerID, groundedStagedLines())
	if err != nil {
		t.Fatalf("add staged offer lines: %v", err)
	}
	// The first staged line is 1 × 200.00 @19% → it adds 20000 / 3800 to the
	// baseline's 10000 / 1900 the moment a human accepts it.
	lineID := ids.UUID(staged[0].Id)
	if _, err := e.Deals.AcceptOfferLineItem(ctx, offerID, lineID); err != nil {
		t.Fatalf("accept offer line: %v", err)
	}

	before, after, evidence := latestOfferUpdateAudit(t, e, offerID)
	assertOfferMoneyImage(t, before, "before", 10000, 1900, 11900)
	assertOfferMoneyImage(t, after, "after", 30000, 5700, 35700)
	assertLineEvidence(t, evidence, lineID)
	if got, _ := lineEvidenceSide(t, evidence, "line_before")["proposal_state"].(string); got != string(deals.ProposalStaged) {
		t.Errorf("evidence line_before = %+v; want proposal_state %q", evidence["line_before"], deals.ProposalStaged)
	}
	if got, _ := lineEvidenceSide(t, evidence, "line_after")["proposal_state"].(string); got != string(deals.ProposalAccepted) {
		t.Errorf("evidence line_after = %+v; want proposal_state %q", evidence["line_after"], deals.ProposalAccepted)
	}
}

// Removing the only line empties the offer's money, and the audit row is the
// only place the numbers it took away survive.
func TestRemoveOfferLineItemAuditsTheTotalsItTookAway(t *testing.T) {
	e := Setup(t)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, offerDeskPerms)
	offerID := baseDraftOffer(ctx, t, e, "Line-remove audit deal")
	lineID := soleLineID(ctx, t, e, offerID)

	if _, err := e.Deals.RemoveOfferLineItem(ctx, offerID, lineID); err != nil {
		t.Fatalf("remove offer line: %v", err)
	}

	before, after, evidence := latestOfferUpdateAudit(t, e, offerID)
	assertOfferMoneyImage(t, before, "before", 10000, 1900, 11900)
	assertOfferMoneyImage(t, after, "after", 0, 0, 0)
	assertLineEvidence(t, evidence, lineID)
	if removed, ok := evidence["line_removed"].(bool); !ok || !removed {
		t.Errorf("evidence = %+v; want line_removed true — that the line is gone is what evidence is for", evidence)
	}
}
