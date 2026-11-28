// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Creating a product: the store-opened entry point and the caller-opened one,
// which share the validation and the insert.

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

// CreateProduct inserts the product and its audit row in the store's own
// transaction. Use CreateProductTx when the write must share a caller-opened one.
func (s *Store) CreateProduct(ctx context.Context, in CreateProductInput) (crmcontracts.Product, error) {
	plan, by, err := s.prepareProductWrite(ctx, in)
	if err != nil {
		return crmcontracts.Product{}, err
	}
	var out crmcontracts.Product
	err = s.Tx(ctx, func(tx pgx.Tx) (err error) {
		out, err = createProductIn(ctx, tx, plan, by)
		return err
	})
	return out, err
}

// CreateProductTx is CreateProduct on the caller's transaction, for a write that
// must commit or roll back with the caller's own.
func (s *Store) CreateProductTx(ctx context.Context, tx pgx.Tx, in CreateProductInput) (crmcontracts.Product, error) {
	plan, by, err := s.prepareProductWrite(ctx, in)
	if err != nil {
		return crmcontracts.Product{}, err
	}
	return createProductIn(ctx, tx, plan, by)
}

// prepareProductWrite is what a create settles before any transaction: the
// grant, who captures the product, and the validated input.
func (s *Store) prepareProductWrite(ctx context.Context, in CreateProductInput) (ProductCreationPreview, string, error) {
	if err := auth.Require(ctx, "product", principal.ActionCreate); err != nil {
		return ProductCreationPreview{}, "", err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return ProductCreationPreview{}, "", err
	}
	plan, err := prepareProductCreation(in)
	return plan, by, err
}

func createProductIn(ctx context.Context, tx pgx.Tx, plan ProductCreationPreview, by string) (crmcontracts.Product, error) {
	in := plan.Input
	if err := refuseClaimedProductSKU(ctx, tx, in.SKU); err != nil {
		return crmcontracts.Product{}, err
	}
	id := ids.New[ids.ProductKind]()
	_, err := tx.Exec(ctx,
		`INSERT INTO product (id, name, sku, description, unit, unit_price_minor,
		                      currency, default_tax_rate, active, source, captured_by,
		                      billing_model, billing_interval_months, component_kind, tax_category)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
		id, in.Name, in.SKU, in.Description, *in.Unit,
		in.UnitPriceMinor, in.Currency, plan.TaxRate, *in.Active, in.Source, by,
		in.BillingModel, in.BillingIntervalMonths, in.ComponentKind, in.TaxCategory)
	if err != nil {
		if storekit.IsUniqueViolation(err) {
			// uq_product_sku: the SKU already names a live product.
			return crmcontracts.Product{}, fmt.Errorf("sku already in use by a live product: %w", apperrors.ErrConflict)
		}
		return crmcontracts.Product{}, fmt.Errorf("insert product: %w", err)
	}
	if _, err := storekit.Audit(ctx, tx, "create", "product", id.UUID, nil, map[string]any{"name": in.Name}); err != nil {
		return crmcontracts.Product{}, fmt.Errorf("audit product create: %w", err)
	}
	out, err := readProduct(ctx, tx, id, storekit.LiveOnly)
	if err != nil {
		return crmcontracts.Product{}, fmt.Errorf("read created product: %w", err)
	}
	return out, nil
}
