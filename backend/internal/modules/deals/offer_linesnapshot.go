// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// What an offer line copies from the product it names, and what it fills in
// when it names none.
//
// Its own file because it is one idea with one rule behind it: the snapshot is
// taken ONCE, at line creation, and a later product edit never reaches the line
// (B-E03.17). An offer that has been sent is a document, and a document does
// not change because somebody repriced or reclassified the catalogue behind it.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// zeroPct is the stored default for a percentage nobody supplied, at the
// numeric(5,2) scale the columns hold.
const zeroPct = "0.00"

// resolveProductSnapshot fills a line's description/unit/price/tax defaults
// from its product; a line with no product keeps the caller's values. The
// snapshot is copied ONCE, here — a later product edit never touches the
// line (B-E03.17). The rate-card price only carries over when the
// currencies agree; a silent conversion would fabricate a number.
func resolveProductSnapshot(ctx context.Context, tx pgx.Tx, productID *ids.ProductID, offerCurrency string, in lineSnapshotDefaults) (lineSnapshotDefaults, error) {
	if productID == nil {
		return in, nil
	}
	var pName, pUnit, pCurrency, pTax string
	var pPrice int64
	var pBillingModel *string
	var pIntervalMonths *int
	err := tx.QueryRow(ctx,
		`SELECT name, unit, currency, default_tax_rate::text, unit_price_minor,
		        billing_model, billing_interval_months
		 FROM product WHERE id = $1 AND archived_at IS NULL`, *productID).
		Scan(&pName, &pUnit, &pCurrency, &pTax, &pPrice, &pBillingModel, &pIntervalMonths)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return lineSnapshotDefaults{}, apperrors.ErrNotFound
		}
		return lineSnapshotDefaults{}, fmt.Errorf("read product for snapshot: %w", err)
	}
	if in.Description == nil {
		in.Description = &pName
	}
	if in.Unit == nil {
		in.Unit = &pUnit
	}
	if in.Price == nil {
		if pCurrency != offerCurrency {
			return lineSnapshotDefaults{}, &ProductCurrencyMismatchError{Product: pCurrency, Offer: offerCurrency}
		}
		in.Price = &pPrice
	}
	if in.TaxRate == nil {
		in.TaxRate = &pTax
	}
	// The classification copies as a PAIR or not at all. Taking the model from
	// the product while keeping a cadence the caller supplied — or the reverse
	// — would compose a classification neither of them stated.
	if in.BillingModel == nil && in.IntervalMonths == nil {
		in.BillingModel, in.IntervalMonths = pBillingModel, pIntervalMonths
	}
	return in, nil
}

// normalizeLineDefaults resolves unit/discount/tax to their stored defaults
// when neither the caller nor the product snapshot supplied a value.
func normalizeLineDefaults(unit, discountPct, taxRate *string) (unitVal, discount, tax string) {
	unitVal = defaultUnit
	if unit != nil && values.HasVisibleText(*unit) {
		unitVal = canonicalUnit(strings.TrimSpace(*unit))
	}
	discount = zeroPct
	if discountPct != nil {
		discount = *discountPct
	}
	tax = zeroPct
	if taxRate != nil {
		tax = *taxRate
	}
	return unitVal, discount, tax
}

func resolveOfferLine(ctx context.Context, tx pgx.Tx, offerCurrency string, in OfferLineInputRow) (resolvedOfferLine, error) {
	defaults, err := resolveProductSnapshot(ctx, tx, in.ProductID, offerCurrency, lineSnapshotDefaults{
		Description: in.Description, Unit: in.Unit, Price: in.UnitPriceMinor, TaxRate: in.TaxRate,
		BillingModel: in.BillingModel, IntervalMonths: in.BillingIntervalMonths,
	})
	if err != nil {
		return resolvedOfferLine{}, err
	}
	if defaults.Description == nil || !values.HasVisibleText(*defaults.Description) {
		return resolvedOfferLine{}, &RequiredFieldError{Field: descriptionColumn}
	}
	if defaults.Price == nil {
		return resolvedOfferLine{}, &RequiredFieldError{Field: "unit_price_minor"}
	}
	if err := checkLinePosition(in.Position); err != nil {
		return resolvedOfferLine{}, err
	}
	unitVal, discount, tax := normalizeLineDefaults(defaults.Unit, in.DiscountPct, defaults.TaxRate)
	line := OfferLineInput{Quantity: in.Quantity, UnitPriceMinor: *defaults.Price, DiscountPct: discount, TaxRate: tax}
	// Validate the figures before the row lands: a number out of range or a
	// malformed decimal answers 422 naming it here, not a CHECK 500 later.
	if err := checkLineFigures(line); err != nil {
		return resolvedOfferLine{}, err
	}
	if _, err := LineTotals(line); err != nil {
		return resolvedOfferLine{}, err
	}
	// The classification is settled here, before the row lands, so a
	// half-stated one earns a field to fix rather than oli_billing_shape.
	if err := validBillingShape(defaults.BillingModel, defaults.IntervalMonths); err != nil {
		return resolvedOfferLine{}, err
	}
	if err := validIntervalCount(defaults.BillingModel, in.IntervalCount); err != nil {
		return resolvedOfferLine{}, err
	}
	return resolvedOfferLine{
		Description: *defaults.Description, Unit: unitVal, Price: *defaults.Price, Discount: discount, Tax: tax,
		BillingModel: defaults.BillingModel, IntervalMonths: defaults.IntervalMonths,
		IntervalCount: in.IntervalCount,
	}, nil
}
