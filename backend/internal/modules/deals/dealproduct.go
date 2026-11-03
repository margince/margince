// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const dealProductTable = "deal_product"

// syncDealProductsFromOffer rewrites the deal's `offer` allocations from the
// accepted offer's lines that name a product. Each product gets one row with
// the lines' net value in the offer's currency. A `manual` row set by hand
// stays, because it is the human's answer and the offer never overrides it.
func syncDealProductsFromOffer(ctx context.Context, tx pgx.Tx, dealID ids.DealID, offer crmcontracts.Offer) error {
	var manual bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM deal_product WHERE deal_id = $1 AND source = 'manual')`, dealID).Scan(&manual); err != nil {
		return fmt.Errorf("read manual deal products: %w", err)
	}
	if manual {
		return nil
	}
	lines, err := readOfferLines(ctx, tx, ids.From[ids.OfferKind](ids.UUID(offer.Id)))
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM deal_product WHERE deal_id = $1 AND source = 'offer'`, dealID); err != nil {
		return fmt.Errorf("clear offer deal products: %w", err)
	}
	netByProduct := map[openapi_types.UUID]int64{}
	lineByProduct := map[openapi_types.UUID]openapi_types.UUID{}
	var order []openapi_types.UUID
	for _, line := range lines {
		if line.ProductId == nil || line.LineNetMinor == nil {
			continue
		}
		if _, seen := netByProduct[*line.ProductId]; !seen {
			order = append(order, *line.ProductId)
			lineByProduct[*line.ProductId] = line.Id
		}
		netByProduct[*line.ProductId] += *line.LineNetMinor
	}
	for _, productID := range order {
		lineID := lineByProduct[productID]
		if _, err := tx.Exec(ctx, `INSERT INTO deal_product (deal_id, product_id, source, offer_line_item_id, amount_minor, currency, component_kind)
			 SELECT $1, $2, 'offer', $3, $4, $5, p.component_kind FROM product p WHERE p.id = $2`,
			dealID, productID, lineID, netByProduct[productID], offer.Currency); err != nil {
			return fmt.Errorf("write offer deal product: %w", err)
		}
	}
	return nil
}

// ReplaceDealProducts sets the deal's products by hand. The rows replace
// every allocation the deal had, offer-derived ones included, and the next
// accepted offer leaves them alone.
func (s *Store) ReplaceDealProducts(ctx context.Context, dealID ids.DealID, in []crmcontracts.DealProductInput) ([]crmcontracts.DealProduct, error) {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return nil, err
	}
	if len(in) > 50 {
		return nil, httperr.Validation("products", "out_of_range", "name at most fifty products on one deal")
	}
	seen := map[openapi_types.UUID]bool{}
	for _, row := range in {
		if seen[row.ProductId] {
			return nil, httperr.Validation("products", "duplicate", "name each product once")
		}
		seen[row.ProductId] = true
		if row.AmountMinor != nil && *row.AmountMinor < 0 {
			return nil, httperr.Validation("products", "invalid", "an allocated amount is zero or more")
		}
	}
	var out []crmcontracts.DealProduct
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisibleLive(ctx, tx, dealTable, dealID.UUID); err != nil {
			return err
		}
		before, err := readDealProducts(ctx, tx, dealID)
		if err != nil {
			return err
		}
		var currency *string
		if err := tx.QueryRow(ctx, `SELECT currency FROM deal WHERE id = $1`, dealID).Scan(&currency); err != nil {
			return fmt.Errorf("read deal currency: %w", err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM deal_product WHERE deal_id = $1`, dealID); err != nil {
			return fmt.Errorf("clear deal products: %w", err)
		}
		for _, row := range in {
			amount, amountCurrency := row.AmountMinor, currency
			if amount == nil || amountCurrency == nil {
				amount, amountCurrency = nil, nil
			}
			tag, err := tx.Exec(ctx, `INSERT INTO deal_product (deal_id, product_id, source, amount_minor, currency, component_kind)
				 SELECT $1, $2, 'manual', $3, $4, p.component_kind FROM product p WHERE p.id = $2 AND p.archived_at IS NULL`,
				dealID, row.ProductId, amount, amountCurrency)
			if err != nil {
				return fmt.Errorf("write manual deal product: %w", err)
			}
			if tag.RowsAffected() == 0 {
				return httperr.Validation("products", "unknown", "choose a live product")
			}
		}
		out, err = readDealProducts(ctx, tx, dealID)
		if err != nil {
			return err
		}
		auditID, err := storekit.Audit(ctx, tx, "update", dealTable, dealID.UUID,
			map[string]any{"products": productIDs(before)}, map[string]any{"products": productIDs(out)})
		if err != nil {
			return fmt.Errorf("audit deal products: %w", err)
		}
		return storekit.EmitEvent(ctx, tx, auditID, dealID.UUID, crmcontracts.PublicEventDealUpdated{ChangedFields: map[string]any{"products": productIDs(out)}})
	})
	return out, err
}

// ListDealProducts reads a deal's allocations under the reader's row scope.
func (s *Store) ListDealProducts(ctx context.Context, dealID ids.DealID) ([]crmcontracts.DealProduct, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []crmcontracts.DealProduct
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, dealTable, dealID.UUID); err != nil {
			return err
		}
		var err error
		out, err = readDealProducts(ctx, tx, dealID)
		return err
	})
	return out, err
}

func readDealProducts(ctx context.Context, tx pgx.Tx, dealID ids.DealID) ([]crmcontracts.DealProduct, error) {
	rows, err := tx.Query(ctx, `SELECT dp.product_id, p.name, dp.source, dp.amount_minor, dp.currency, dp.component_kind, dp.updated_at
		 FROM deal_product dp JOIN product p ON p.id = dp.product_id WHERE dp.deal_id = $1 ORDER BY p.name, dp.product_id`, dealID)
	if err != nil {
		return nil, fmt.Errorf("read deal products: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (crmcontracts.DealProduct, error) {
		var product crmcontracts.DealProduct
		var source, kind *string
		err := row.Scan(&product.ProductId, &product.ProductName, &source, &product.AmountMinor, &product.Currency, &kind, &product.UpdatedAt)
		if source != nil {
			product.Source = crmcontracts.DealProductSource(*source)
		}
		if kind != nil {
			value := crmcontracts.DealProductComponentKind(*kind)
			product.ComponentKind = &value
		}
		return product, err
	})
	if err != nil {
		return nil, fmt.Errorf("scan deal products: %w", err)
	}
	if out == nil {
		out = []crmcontracts.DealProduct{}
	}
	return out, nil
}

func productIDs(products []crmcontracts.DealProduct) []string {
	out := make([]string, 0, len(products))
	for _, product := range products {
		out = append(out, product.ProductId.String())
	}
	return out
}
