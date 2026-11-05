// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// What a deal line copies from its product, and what makes a line valid.
// The product snapshot, the tax-category rule and the billing-shape rule are
// the offer line's own functions, so a deal line and the offer line made from
// it are judged by one rule.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// maxLineDescription is the longest description deal_line_item stores.
const maxLineDescription = 2000

// resolveNewDealLine builds a line from the request and, for a product line,
// the product's snapshot. A product priced in another currency needs a typed
// price (ProductCurrencyMismatchError), the offer line's rule.
func (s *Store) resolveNewDealLine(ctx context.Context, tx pgx.Tx, currency string, in AddDealLineInput) (dealLineRow, error) {
	if err := checkLineTaxCategory(in.TaxCategory, in.TaxRate); err != nil {
		return dealLineRow{}, err
	}
	defaults := lineSnapshotDefaults{
		Description: in.Description, Unit: in.Unit, Price: in.UnitPriceMinor, TaxRate: in.TaxRate,
		TaxCategory: in.TaxCategory, BillingModel: in.BillingModel, IntervalMonths: in.IntervalMonths,
	}
	line := dealLineRow{ID: ids.NewV7(), Currency: currency, Quantity: in.Quantity, DiscountPct: zeroPct, IntervalCount: in.IntervalCount}
	if in.DiscountPct != nil {
		line.DiscountPct = *in.DiscountPct
	}
	if in.ProductID != nil {
		var err error
		if defaults, err = resolveProductSnapshot(ctx, tx, in.ProductID, currency, "", defaults); err != nil {
			if errors.Is(err, apperrors.ErrNotFound) {
				return dealLineRow{}, auth.NameMissingReference("product_id", err)
			}
			return dealLineRow{}, err
		}
		product := in.ProductID.UUID
		line.ProductID = &product
		if err := tx.QueryRow(ctx, `SELECT name, sku, component_kind FROM product WHERE id = $1`, product).
			Scan(&line.CatalogName, &line.SKU, &line.ComponentKind); err != nil {
			return dealLineRow{}, fmt.Errorf("read the product's catalogue identity: %w", err)
		}
	}
	if defaults.Description != nil {
		line.Description = strings.TrimSpace(*defaults.Description)
	}
	line.Unit, _, _ = normalizeLineDefaults(defaults.Unit, nil, nil)
	line.UnitPriceMinor = defaults.Price
	line.BillingModel, line.IntervalMonths = defaults.BillingModel, defaults.IntervalMonths
	line.TaxCategory, line.TaxRate = defaults.TaxCategory, defaults.TaxRate
	if line.TaxCategory == nil && line.TaxRate == nil {
		line.TaxRate = new(zeroPct)
	}
	return line, s.checkDealLine(ctx, tx, line)
}

// patchDealLine applies a sparse edit to a line and judges the result.
func (s *Store) patchDealLine(ctx context.Context, tx pgx.Tx, cur dealLineRow, in UpdateDealLineInput) (dealLineRow, error) {
	if err := checkLineTaxCategory(in.TaxCategory, in.TaxRate); err != nil {
		return dealLineRow{}, err
	}
	next := cur
	if in.Description != nil {
		next.Description = strings.TrimSpace(*in.Description)
	}
	if in.Unit != nil {
		next.Unit, _, _ = normalizeLineDefaults(in.Unit, nil, nil)
	}
	if in.QuantitySet {
		next.Quantity = in.Quantity
	}
	if in.PriceSet {
		next.UnitPriceMinor = in.UnitPriceMinor
	}
	if in.DiscountPct != nil {
		next.DiscountPct = *in.DiscountPct
	}
	if in.TaxCategory != nil {
		next.TaxCategory, next.TaxRate = in.TaxCategory, nil
	}
	if in.TaxRate != nil {
		next.TaxCategory, next.TaxRate = nil, in.TaxRate
	}
	if in.Classified {
		next.BillingModel, next.IntervalMonths = in.BillingModel, in.IntervalMonths
		if in.BillingModel == nil || *in.BillingModel != BillingRecurring {
			// A term belongs to a recurring price only; reclassifying drops it.
			next.IntervalCount = nil
		}
	}
	if in.IntervalCountSet {
		next.IntervalCount = in.IntervalCount
	}
	return next, s.checkDealLine(ctx, tx, next)
}

// checkDealLine refuses a line no deal can hold, naming the field. An unknown
// quantity or price is allowed: it leaves the deal's value incomplete.
func (s *Store) checkDealLine(ctx context.Context, tx pgx.Tx, line dealLineRow) error {
	if !values.HasVisibleText(line.Description) {
		return &RequiredFieldError{Field: descriptionColumn}
	}
	if len(line.Description) > maxLineDescription {
		return &FigureError{Field: descriptionColumn, Rule: fmt.Sprintf("holds at most %d characters", maxLineDescription)}
	}
	if line.Quantity != nil {
		if err := checkFigure(quantityLimit, *line.Quantity); err != nil {
			return err
		}
	}
	if line.UnitPriceMinor != nil {
		if err := checkPrice(*line.UnitPriceMinor); err != nil {
			return err
		}
	}
	if err := checkFigure(discountLimit, line.DiscountPct); err != nil {
		return err
	}
	if line.TaxRate != nil {
		if err := checkFigure(taxLimit, *line.TaxRate); err != nil {
			return err
		}
	}
	if line.TaxCategory != nil {
		// The sheet must hold a rate today, as it must for an offer line.
		if _, err := sheetTaxRate(ctx, tx, s.draftTaxBasis(ctx, tx, nil), *line.TaxCategory); err != nil {
			return err
		}
	}
	if err := validBillingShape(line.BillingModel, line.IntervalMonths); err != nil {
		return err
	}
	if err := validIntervalCount(line.BillingModel, line.IntervalCount); err != nil {
		return err
	}
	_, err := DealLineTotals(DealLineInput{
		Quantity: line.Quantity, UnitPriceMinor: line.UnitPriceMinor, DiscountPct: line.DiscountPct, TaxRate: line.TaxRate,
		BillingModel: line.BillingModel, IntervalMonths: line.IntervalMonths, IntervalCount: line.IntervalCount,
	})
	return err
}

func insertDealLine(ctx context.Context, tx pgx.Tx, dealID ids.DealID, line dealLineRow) error {
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO deal_line_item (id, deal_id, position, product_id, catalog_name, sku, description, unit,
		                             quantity, unit_price_minor, currency, discount_pct, tax_rate, tax_category,
		                             billing_model, billing_interval_months, interval_count, component_kind, captured_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)`,
		line.ID, dealID, line.Position, line.ProductID, line.CatalogName, line.SKU, line.Description, line.Unit,
		line.Quantity, line.UnitPriceMinor, line.Currency, line.DiscountPct, line.TaxRate, line.TaxCategory,
		line.BillingModel, line.IntervalMonths, line.IntervalCount, line.ComponentKind, by); err != nil {
		return fmt.Errorf("insert deal line: %w", err)
	}
	return nil
}

func updateDealLineRow(ctx context.Context, tx pgx.Tx, line dealLineRow) error {
	tag, err := tx.Exec(ctx,
		`UPDATE deal_line_item SET description = $2, unit = $3, quantity = $4, unit_price_minor = $5,
		        discount_pct = $6, tax_rate = $7, tax_category = $8, billing_model = $9,
		        billing_interval_months = $10, interval_count = $11
		  WHERE id = $1 AND version = $12`,
		line.ID, line.Description, line.Unit, line.Quantity, line.UnitPriceMinor, line.DiscountPct, line.TaxRate,
		line.TaxCategory, line.BillingModel, line.IntervalMonths, line.IntervalCount, line.Version)
	if err != nil {
		return fmt.Errorf("update deal line: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &apperrors.VersionSkewError{Message: "The line changed while it was saved. Reload the deal's products and apply your change again."}
	}
	return nil
}

// sameDealLine answers whether an edit left the line as it was, comparing
// decimals by value: "10" and the stored "10.00" are one discount.
func sameDealLine(a, b dealLineRow) bool {
	return a.Description == b.Description && a.Unit == b.Unit &&
		sameDecimal(a.Quantity, b.Quantity) && samePtr(a.UnitPriceMinor, b.UnitPriceMinor) &&
		sameDecimal(&a.DiscountPct, &b.DiscountPct) && sameDecimal(a.TaxRate, b.TaxRate) &&
		samePtr(a.TaxCategory, b.TaxCategory) && samePtr(a.BillingModel, b.BillingModel) &&
		samePtr(a.IntervalMonths, b.IntervalMonths) && samePtr(a.IntervalCount, b.IntervalCount)
}

func samePtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func sameDecimal(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	x, errX := ratFromDecimal("", *a)
	y, errY := ratFromDecimal("", *b)
	if errX != nil || errY != nil {
		return *a == *b
	}
	return x.Cmp(y) == 0
}
