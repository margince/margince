// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// OfferCreationLine retains the resolved product alongside the figures the writer will use.
type OfferCreationLine struct {
	Input    OfferLineInputRow
	Figures  LineFigures
	resolved resolvedOfferLine
}

// OfferCreationPreview keeps line and recurring totals derived from the same resolved inputs.
type OfferCreationPreview struct {
	Input     CreateOfferInput
	Lines     []OfferCreationLine
	Totals    OfferFigures
	Recurring RecurringFigures
}

func planNewOfferLines(ctx context.Context, tx pgx.Tx, currency string, inputs []OfferLineInputRow) ([]OfferCreationLine, OfferFigures, error) {
	var lines []OfferCreationLine
	var amounts []OfferLineInput
	positions := map[int]bool{}
	highest := 0
	for i, in := range inputs {
		line, err := resolveOfferLine(ctx, tx, currency, in)
		if err != nil {
			return nil, OfferFigures{}, fmt.Errorf("line %d: %w", i+1, err)
		}
		position := highest + 1
		if in.Position != nil {
			position = *in.Position
		}
		if err := checkLinePosition(&position); err != nil {
			return nil, OfferFigures{}, err
		}
		if positions[position] {
			return nil, OfferFigures{}, fmt.Errorf("position %d is already taken on this offer: %w", position, apperrors.ErrConflict)
		}
		positions[position] = true
		highest = max(highest, position)
		in.Position, in.Description, in.Unit = &position, &line.Description, &line.Unit
		in.UnitPriceMinor, in.DiscountPct, in.TaxRate = &line.Price, &line.Discount, &line.Tax
		in.BillingModel, in.BillingIntervalMonths, in.IntervalCount = line.BillingModel, line.IntervalMonths, line.IntervalCount
		amount := OfferLineInput{Quantity: in.Quantity, UnitPriceMinor: line.Price, DiscountPct: line.Discount, TaxRate: line.Tax}
		figures, err := LineTotals(amount)
		if err != nil {
			return nil, OfferFigures{}, err
		}
		lines = append(lines, OfferCreationLine{Input: in, Figures: figures, resolved: line})
		amounts = append(amounts, amount)
	}
	totals, err := OfferTotals(amounts)
	if err != nil {
		return nil, OfferFigures{}, err
	}
	return lines, totals, nil
}

// PreviewOfferCreation shares admission and line pricing with the writer without allocating an offer.
func (s *Store) PreviewOfferCreation(ctx context.Context, dealID ids.DealID, in CreateOfferInput) (OfferCreationPreview, error) {
	var out OfferCreationPreview
	if err := auth.Require(ctx, "offer", principal.ActionCreate); err != nil {
		return out, err
	}
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		in, err = admitOfferCreation(ctx, tx, dealID, in)
		if err != nil {
			return err
		}
		if in.TemplateID != nil {
			if err := auth.Require(ctx, "offer_template", principal.ActionRead); err != nil {
				return err
			}
		}

		for _, line := range in.LineItems {
			if line.ProductID != nil {
				if err := auth.Require(ctx, "product", principal.ActionRead); err != nil {
					return err
				}
			}
		}
		lines, totals, err := planNewOfferLines(ctx, tx, in.Currency, in.LineItems)
		if err != nil {
			return err
		}
		recurring := make([]RecurringLineInput, 0, len(lines))
		for _, line := range lines {
			item := RecurringLineInput{Line: OfferLineInput{Quantity: line.Input.Quantity, UnitPriceMinor: line.resolved.Price, DiscountPct: line.resolved.Discount, TaxRate: line.resolved.Tax}, IntervalMonths: line.Input.BillingIntervalMonths, IntervalCount: line.Input.IntervalCount}
			if line.Input.BillingModel != nil {
				item.BillingModel = *line.Input.BillingModel
			}
			recurring = append(recurring, item)
		}
		figures, err := OfferRecurringTotals(recurring)
		if err != nil {
			return err
		}
		out = OfferCreationPreview{Input: in, Lines: lines, Totals: totals, Recurring: figures}
		return nil
	})
	if err != nil {
		return OfferCreationPreview{}, err
	}
	return out, nil
}

func admitOfferCreation(ctx context.Context, tx pgx.Tx, dealID ids.DealID, in CreateOfferInput) (CreateOfferInput, error) {
	if err := checkCurrency(in.Currency); err != nil {
		return CreateOfferInput{}, err
	}
	if err := auth.EnsureLinkTarget(ctx, tx, dealTable, dealID.UUID); err != nil {
		return CreateOfferInput{}, err
	}
	buyer, err := resolveBuyerCompany(ctx, tx, dealID, in.BuyerCompanyID)
	if err != nil {
		return CreateOfferInput{}, err
	}
	if buyer != nil {
		if err := auth.EnsureLinkTarget(ctx, tx, "company", buyer.UUID); err != nil {
			return CreateOfferInput{}, err
		}
	}
	if err := resolveOfferTemplateRef(ctx, tx, in.TemplateID); err != nil {
		return CreateOfferInput{}, err
	}
	in.BuyerCompanyID = buyer
	return in, nil
}
