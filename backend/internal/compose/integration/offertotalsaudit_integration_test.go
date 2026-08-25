// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What a line write records about the OFFER (E03.17/E03.21a). Adding a line
// and staging a batch both audit an update to the offer, and the only field
// transition either one makes on that record is the three stored money
// columns: an audit row that cannot say what they held before renders half a
// change in field history and leaves nothing to restore the numbers from.
// The count of lines added is context ABOUT the write, not a field the offer
// has, so it belongs in evidence — an image carrying it puts a change on the
// screen that never happened to the record.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/modules/deals"
	"github.com/gradionhq/margince/backend/internal/platform/database"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// offerMoneyImage is one side of an offer update's audit images: the three
// money columns, plus the two operation-metadata keys that must never appear
// in one. A non-nil metadata field here IS the defect.
type offerMoneyImage struct {
	NetMinor         *int64 `json:"net_minor"`
	TaxMinor         *int64 `json:"tax_minor"`
	GrossMinor       *int64 `json:"gross_minor"`
	LineAdded        *bool  `json:"line_added"`
	StagedLinesAdded *int   `json:"staged_lines_added"`
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
	for key, present := range map[string]bool{"line_added": img.LineAdded != nil, "staged_lines_added": img.StagedLinesAdded != nil} {
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
