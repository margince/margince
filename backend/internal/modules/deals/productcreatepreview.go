// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ProductCreationPreview keeps the admitted tax spelling with the normalized creation input.
type ProductCreationPreview struct {
	Input   CreateProductInput
	TaxRate string
}

func prepareProductCreation(in CreateProductInput) (ProductCreationPreview, error) {
	name, err := httperr.RequireNonBlank("name", in.Name)
	if err != nil {
		return ProductCreationPreview{}, err
	}
	in.Name = name
	unit, _, _ := normalizeLineDefaults(in.Unit, nil, nil)
	in.Unit = &unit
	tax := zeroPct
	if in.DefaultTaxRate != nil {
		if err := checkFigure(defaultTaxLimit, wireDecimal(*in.DefaultTaxRate)); err != nil {
			return ProductCreationPreview{}, err
		}
		tax = formatPct(*in.DefaultTaxRate)
	}
	if err := checkPrice(in.UnitPriceMinor); err != nil {
		return ProductCreationPreview{}, err
	}
	if err := checkCurrency(in.Currency); err != nil {
		return ProductCreationPreview{}, err
	}
	active := in.Active == nil || *in.Active
	in.Active = &active
	if err := validBillingShape(in.BillingModel, in.BillingIntervalMonths); err != nil {
		return ProductCreationPreview{}, err
	}
	return ProductCreationPreview{Input: in, TaxRate: tax}, nil
}

// PreviewProductCreation applies catalog admission without reserving a product row.
func (s *Store) PreviewProductCreation(ctx context.Context, in CreateProductInput) (ProductCreationPreview, error) {
	if err := auth.Require(ctx, "product", principal.ActionCreate); err != nil {
		return ProductCreationPreview{}, err
	}
	plan, err := prepareProductCreation(in)
	if err != nil {
		return ProductCreationPreview{}, err
	}
	err = s.Tx(ctx, func(tx pgx.Tx) error { return refuseClaimedProductSKU(ctx, tx, in.SKU) })
	if err != nil {
		return ProductCreationPreview{}, err
	}
	return plan, nil
}

func refuseClaimedProductSKU(ctx context.Context, tx pgx.Tx, sku *string) error {
	if sku == nil {
		return nil
	}
	var claimed bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM product WHERE sku=@sku AND archived_at IS NULL)`, pgx.NamedArgs{productSkuColumn: *sku}).Scan(&claimed); err != nil {
		return err
	}
	if claimed {
		return fmt.Errorf("sku already in use by a live product: %w", apperrors.ErrConflict)
	}
	return nil
}
