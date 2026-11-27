// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
	if in.TaxCategory != nil {
		if err := checkTaxCategory(*in.TaxCategory); err != nil {
			return ProductCreationPreview{}, err
		}
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

// RefuseProductUpdate answers the refusals UpdateProduct would answer from the
// stored product and the input alone. Those are a value outside its range and a
// billing shape the pair of billing fields would break. It writes nothing, so the
// import's test run says what the commit will.
func RefuseProductUpdate(current crmcontracts.Product, in UpdateProductInput) error {
	_, err := buildProductPatch(current, in)
	return err
}

// ProductHoldingSKU reports whether a live product other than `self` holds the
// SKU, which a create or an update of that SKU is refused over.
func ProductHoldingSKU(ctx context.Context, tx pgx.Tx, sku string, self ids.UUID) (bool, error) {
	var held bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM product WHERE sku = @sku AND id <> @self AND archived_at IS NULL)`,
		pgx.NamedArgs{productSkuColumn: sku, "self": self}).Scan(&held)
	return held, err
}

// LockProductLive takes the row lock a reviewed import holds while it checks the
// product's version, for a product that is not archived.
func (s *Store) LockProductLive(ctx context.Context, tx pgx.Tx, id ids.ProductID) error {
	if err := auth.Require(ctx, "product", principal.ActionRead); err != nil {
		return err
	}
	var locked bool
	err := tx.QueryRow(ctx, `SELECT true FROM product WHERE id = $1 AND archived_at IS NULL FOR UPDATE`, id).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock product %s: %w", id, err)
	}
	return nil
}
