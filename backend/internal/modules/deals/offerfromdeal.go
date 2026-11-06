// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// An offer made from a deal's lines copies them at one commercial revision
// and names it. A later deal change shows as a newer revision and never
// reaches the offer by itself; only a refresh of the draft takes it.
//
// The legal entity and the language come from the offer request, as for any
// offer. A product line takes its name in the offer's language when the
// seller left the catalogue name as it was.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// fromDealRevisionField names the request field that asks for the deal's lines.
const fromDealRevisionField = "from_deal_revision"

// OfferLinesFromDealError refuses a line edit on an offer whose lines come
// from its deal.
type OfferLinesFromDealError struct{}

func (e *OfferLinesFromDealError) Error() string {
	return "this offer's lines come from its deal; change the deal's products, then refresh the draft from the deal"
}

// FieldFault points at the lines.
func (e *OfferLinesFromDealError) FieldFault() (field, code, message string) {
	return lineItemsField, "offer_lines_from_deal", e.Error()
}

// DealHasNoLinesError refuses a request to copy lines from a deal without any.
type DealHasNoLinesError struct{}

func (e *DealHasNoLinesError) Error() string {
	return "this deal is not priced from product lines; add the lines to the offer itself"
}

// FieldFault points at the field that asked for the deal's lines.
func (e *DealHasNoLinesError) FieldFault() (field, code, message string) {
	return fromDealRevisionField, "deal_has_no_lines", e.Error()
}

// DealPricingIncompleteError refuses an offer from a deal whose lines still
// lack a quantity or a price: a document cannot state an unknown.
type DealPricingIncompleteError struct{}

func (e *DealPricingIncompleteError) Error() string {
	return "a line of this deal has no quantity or price yet; complete the deal's products first"
}

// MessageFault names the condition; the fields are on the deal, not here.
func (e *DealPricingIncompleteError) MessageFault() (code, message string) {
	return "pricing_incomplete", e.Error()
}

// ensureTypedDraft admits a line edit: the offer is a draft, and its lines
// were typed on it rather than copied from its deal.
func ensureTypedDraft(offer crmcontracts.Offer) error {
	if err := ensureDraft(offer); err != nil {
		return err
	}
	return ensureOfferLinesTyped(offer)
}

// ensureOfferLinesTyped refuses a line edit on an offer made from its deal.
func ensureOfferLinesTyped(offer crmcontracts.Offer) error {
	if offer.SourceCommercialRevision != nil {
		return &OfferLinesFromDealError{}
	}
	return nil
}

// admitOfferLines decides where a new offer's lines come from. A deal in
// `lines` mode hands over its lines at the named revision and refuses typed
// ones; any other deal refuses the revision. It answers the revision copied,
// nil when the lines are the request's own. It takes the deal's row lock,
// so the lines cannot change between this read and the copy.
func admitOfferLines(ctx context.Context, tx pgx.Tx, dealID ids.DealID, in CreateOfferInput) (CreateOfferInput, []ids.UUID, *int64, error) {
	if _, err := storekit.LockRow(ctx, tx, dealTable, dealID.UUID, storekit.LiveOnly); err != nil {
		return CreateOfferInput{}, nil, nil, err
	}
	// An offer prices the deal it hangs off, so it takes the deal's write grant.
	if err := auth.EnsureWritable(ctx, tx, dealTable, dealID.UUID); err != nil {
		return CreateOfferInput{}, nil, nil, err
	}
	head, err := readDealCommercialHead(ctx, tx, dealID)
	if err != nil {
		return CreateOfferInput{}, nil, nil, err
	}
	if head.Mode != modeLines {
		if in.FromDealRevision != nil {
			return CreateOfferInput{}, nil, nil, &DealHasNoLinesError{}
		}
		return in, nil, nil, nil
	}
	if len(in.LineItems) > 0 {
		return CreateOfferInput{}, nil, nil, &LineOwnedValueError{Field: lineItemsField}
	}
	if in.FromDealRevision == nil {
		return CreateOfferInput{}, nil, nil, &RequiredFieldError{Field: fromDealRevisionField}
	}
	rows, sources, err := dealLinesAsOfferLines(ctx, tx, dealID, head, *in.FromDealRevision, in.Currency)
	if err != nil {
		return CreateOfferInput{}, nil, nil, err
	}
	in.LineItems = rows
	return in, sources, &head.Revision, nil
}

// dealLinesAsOfferLines turns the deal's lines at revision into offer lines,
// with the deal line each one came from.
func dealLinesAsOfferLines(ctx context.Context, tx pgx.Tx, dealID ids.DealID, head dealCommercialHead, revision int64, currency string) ([]OfferLineInputRow, []ids.UUID, error) {
	if head.Revision != revision {
		return nil, nil, staleRevision(revision, head.Revision)
	}
	readable, err := dealAmountReadable(ctx, tx, dealID)
	if err != nil {
		return nil, nil, err
	}
	if !readable {
		return nil, nil, apperrors.ErrPermissionDenied
	}
	lines, err := readDealLineRows(ctx, tx, dealID)
	if err != nil {
		return nil, nil, err
	}
	if len(lines) == 0 {
		return nil, nil, &DealPricingIncompleteError{}
	}
	rows := make([]OfferLineInputRow, 0, len(lines))
	sources := make([]ids.UUID, 0, len(lines))
	for i, line := range lines {
		if line.Quantity == nil || line.UnitPriceMinor == nil {
			return nil, nil, &DealPricingIncompleteError{}
		}
		if line.Currency != currency {
			return nil, nil, &LineCurrencyError{Lines: line.Currency, Asked: currency}
		}
		rows = append(rows, offerLineFromDealLine(i+1, line))
		sources = append(sources, line.ID)
	}
	return rows, sources, nil
}

func offerLineFromDealLine(position int, line dealLineRow) OfferLineInputRow {
	row := OfferLineInputRow{
		Position: &position, Unit: &line.Unit, Quantity: *line.Quantity, UnitPriceMinor: line.UnitPriceMinor,
		DiscountPct: &line.DiscountPct, TaxRate: line.TaxRate, TaxCategory: line.TaxCategory,
		BillingModel: line.BillingModel, BillingIntervalMonths: line.IntervalMonths, IntervalCount: line.IntervalCount,
	}
	description := line.Description
	row.Description = &description
	if line.ProductID != nil {
		product := ids.From[ids.ProductKind](*line.ProductID)
		row.ProductID = &product
		if line.CatalogName != nil && *line.CatalogName == line.Description {
			// Unedited: the offer names the product in its own language.
			row.Description = nil
		}
	}
	return row
}

// bindOfferLinesToDeal records which deal line each offer line came from. The
// lines were inserted at positions 1..n in the deal's order.
func bindOfferLinesToDeal(ctx context.Context, tx pgx.Tx, offerID ids.OfferID, sources []ids.UUID) error {
	for i, source := range sources {
		if _, err := tx.Exec(ctx,
			`UPDATE offer_line_item SET source_deal_line_id = $3 WHERE offer_id = $1 AND position = $2`,
			offerID, i+1, source); err != nil {
			return fmt.Errorf("name the deal line an offer line came from: %w", err)
		}
	}
	return nil
}

// RefreshOfferFromDeal replaces a draft's lines with the deal's lines at the
// revision the caller reviewed. The header, number and language stay.
func (s *Store) RefreshOfferFromDeal(ctx context.Context, offerID ids.OfferID, expectedRevision int64, ifVersion *int64) (crmcontracts.Offer, error) {
	if err := auth.Require(ctx, "offer", principal.ActionUpdate); err != nil {
		return crmcontracts.Offer{}, err
	}
	var out crmcontracts.Offer
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		current, lock, err := visibleOfferLocked(ctx, tx, offerID, storekit.LiveOnly)
		if err != nil {
			return err
		}
		if err := ensureDraft(current); err != nil {
			return err
		}
		if current.SourceCommercialRevision == nil {
			return &DealHasNoLinesError{}
		}
		if err := storekit.EnsureIfMatch(ctx, tx, "offer", offerID.UUID, ifVersion); err != nil {
			return err
		}
		dealID := ids.From[ids.DealKind](ids.UUID(current.DealId))
		head, err := readDealCommercialHead(ctx, tx, dealID)
		if err != nil {
			return err
		}
		if head.Mode != modeLines {
			return &DealHasNoLinesError{}
		}
		rows, sources, err := dealLinesAsOfferLines(ctx, tx, dealID, head, expectedRevision, current.Currency)
		if err != nil {
			return err
		}
		if err := s.replaceDraftLines(ctx, tx, current, rows, sources); err != nil {
			return err
		}
		bound := storekit.NewPatch()
		bound.Set("source_commercial_revision", *current.SourceCommercialRevision, head.Revision)
		if !bound.Empty() {
			if err := bound.ApplyLocked(ctx, tx, lock); err != nil {
				return fmt.Errorf("name the deal revision the draft came from: %w", err)
			}
		}
		totals, err := recomputeOfferTotals(ctx, tx, offerID)
		if err != nil {
			return err
		}
		if _, err := storekit.AuditWithEvidence(ctx, tx, "update", "offer", offerID.UUID,
			offerTotalsImage(totals.Before), offerTotalsImage(totals.After),
			map[string]any{"refreshed_from_revision": head.Revision, "was_revision": *current.SourceCommercialRevision}); err != nil {
			return fmt.Errorf("audit the refresh from the deal: %w", err)
		}
		out, err = readOfferWithLines(ctx, tx, offerID, storekit.LiveOnly)
		return err
	})
	return out, err
}

// replaceDraftLines swaps the draft's lines for rows and names the deal line
// each came from.
func (s *Store) replaceDraftLines(ctx context.Context, tx pgx.Tx, current crmcontracts.Offer, rows []OfferLineInputRow, sources []ids.UUID) error {
	offerID := ids.From[ids.OfferKind](ids.UUID(current.Id))
	if _, err := tx.Exec(ctx, `DELETE FROM offer_line_item WHERE offer_id = $1`, offerID); err != nil {
		return fmt.Errorf("clear the draft's lines: %w", err)
	}
	basis := s.draftTaxBasis(ctx, tx, legalEntityIDOf(current))
	if err := insertOfferLines(ctx, tx, offerID, current.Currency, languageOf(current.Language), basis, rows); err != nil {
		return err
	}
	return bindOfferLinesToDeal(ctx, tx, offerID, sources)
}

// ensureLinesPricedToWin refuses to win a deal priced from lines that are not
// complete: the won value would be a number nobody stated. A deal priced
// otherwise keeps the rules it had.
func ensureLinesPricedToWin(ctx context.Context, tx pgx.Tx, dealID ids.DealID) error {
	head, err := readDealCommercialHead(ctx, tx, dealID)
	if err != nil {
		return err
	}
	if head.Mode == modeLines && head.AmountMinor == nil {
		return &DealPricingIncompleteError{}
	}
	return nil
}
