// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"
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

// syncDealProductsFromOffer rewrites the deal's `offer` allocations from the
// accepted offer's lines that name a product, one row per product with the
// lines' net value. A `manual` set stays, because it is the human's answer.
// It runs under the deal row lock the amount sync took, so a manual set
// cannot land between the check and the writes. It answers the rows it
// wrote, and nil when the manual set stood.
func syncDealProductsFromOffer(ctx context.Context, tx pgx.Tx, dealID ids.DealID, offer crmcontracts.Offer) ([]crmcontracts.DealProduct, error) {
	var manual bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM deal_product WHERE deal_id = $1 AND source = 'manual')`, dealID).Scan(&manual); err != nil {
		return nil, fmt.Errorf("read manual deal products: %w", err)
	}
	if manual {
		return nil, nil
	}
	lines, err := readOfferLines(ctx, tx, ids.From[ids.OfferKind](ids.UUID(offer.Id)))
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM deal_product WHERE deal_id = $1 AND source = 'offer'`, dealID); err != nil {
		return nil, fmt.Errorf("clear offer deal products: %w", err)
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
			return nil, fmt.Errorf("write offer deal product: %w", err)
		}
	}
	return readDealProducts(ctx, tx, dealID)
}

// ReplaceDealProducts sets the deal's products by hand. The rows replace
// every allocation the deal had, offer-derived ones included, and the next
// accepted offer leaves them alone. An empty set removes the hand-set rows
// and derives the products from the accepted offer again, if there is one.
func (s *Store) ReplaceDealProducts(ctx context.Context, dealID ids.DealID, in []crmcontracts.DealProductInput) ([]crmcontracts.DealProduct, error) {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, "product", principal.ActionRead); err != nil {
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
		if err := auth.EnsureWritableLive(ctx, tx, dealTable, dealID.UUID); err != nil {
			return err
		}
		// The lock orders this set against an offer accept on the same deal, so
		// the two never leave a mixed manual and offer set behind.
		if _, err := storekit.LockRow(ctx, tx, dealTable, dealID.UUID, storekit.LiveOnly); err != nil {
			return fmt.Errorf("lock deal for product set: %w", err)
		}
		before, err := readDealProducts(ctx, tx, dealID)
		if err != nil {
			return err
		}
		currency, dealAmount, err := dealAllocationBound(ctx, tx, dealID)
		if err != nil {
			return err
		}
		if err := refuseOverAllocation(in, dealAmount); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM deal_product WHERE deal_id = $1`, dealID); err != nil {
			return fmt.Errorf("clear deal products: %w", err)
		}
		if len(in) == 0 {
			if err := rederiveDealProducts(ctx, tx, dealID); err != nil {
				return err
			}
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
		auditID, err := storekit.Audit(ctx, tx, "update", "deal", dealID.UUID,
			map[string]any{"products": allocationImage(before)}, map[string]any{"products": allocationImage(out)})
		if err != nil {
			return fmt.Errorf("audit deal products: %w", err)
		}
		return storekit.EmitEvent(ctx, tx, auditID, dealID.UUID, crmcontracts.PublicEventDealUpdated{ChangedFields: map[string]any{"products": allocationImage(out)}})
	})
	return out, err
}

// dealAllocationBound reads the deal's currency and the amount that bounds a
// set, under the field mask. A caller the amount is masked from sets products
// that no amount bounds.
func dealAllocationBound(ctx context.Context, tx pgx.Tx, dealID ids.DealID) (*string, *int64, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, nil, err
	}
	args := []any{dealID}
	amount, err := auth.MaskedExpressionSQL(ctx, "deal", "amount_minor", "deal", "deal.amount_minor", func(value any) int {
		args = append(args, value)
		return len(args)
	})
	if err != nil {
		return nil, nil, err
	}
	var currency *string
	var dealAmount *int64
	if err := tx.QueryRow(ctx, `SELECT deal.currency, `+amount+` FROM deal WHERE deal.id = $1`, args...).Scan(&currency, &dealAmount); err != nil {
		return nil, nil, fmt.Errorf("read deal currency: %w", err)
	}
	return currency, dealAmount, nil
}

// refuseOverAllocation keeps the allocations inside the deal's own amount, so
// product bookings can never add up to more than the deal was won for. An
// unpriced deal bounds nothing yet.
func refuseOverAllocation(in []crmcontracts.DealProductInput, dealAmount *int64) error {
	if dealAmount == nil {
		return nil
	}
	var total int64
	for _, row := range in {
		if row.AmountMinor != nil {
			total += *row.AmountMinor
		}
	}
	if total > *dealAmount {
		return httperr.Validation("products", "exceeds_deal", "the allocated amounts add up to more than the deal's amount")
	}
	return nil
}

// rederiveDealProducts writes the allocations the latest accepted offer
// names, after a hand-set list was cleared.
func rederiveDealProducts(ctx context.Context, tx pgx.Tx, dealID ids.DealID) error {
	var offerID *ids.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM offer WHERE deal_id = $1 AND status = 'accepted' AND archived_at IS NULL ORDER BY accepted_at DESC NULLS LAST LIMIT 1`, dealID).Scan(&offerID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("find the accepted offer: %w", err)
	}
	if offerID == nil {
		return nil
	}
	offer, err := readOfferWithLines(ctx, tx, ids.From[ids.OfferKind](*offerID), storekit.LiveOnly)
	if err != nil {
		return err
	}
	_, err = syncDealProductsFromOffer(ctx, tx, dealID, offer)
	return err
}

// ListDealProducts reads a deal's allocations under the reader's row scope.
func (s *Store) ListDealProducts(ctx context.Context, dealID ids.DealID) ([]crmcontracts.DealProduct, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, "product", principal.ActionRead); err != nil {
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

// allocationImage is what an audit row and a deal.updated event say about the
// allocations: each product with its amount, currency and kind. So a changed
// amount under the same product is a change the trail shows.
func allocationImage(products []crmcontracts.DealProduct) []map[string]any {
	out := make([]map[string]any, 0, len(products))
	for _, product := range products {
		out = append(out, map[string]any{"product_id": product.ProductId.String(), "source": product.Source, "amount_minor": product.AmountMinor, "currency": product.Currency, "component_kind": product.ComponentKind})
	}
	return out
}

// noteAllocations adds the allocations an accept wrote to the deal's changed
// fields, so the accept's audit image and event carry them.
func noteAllocations(dealChanged map[string]any, allocated []crmcontracts.DealProduct) map[string]any {
	if allocated == nil {
		return dealChanged
	}
	if dealChanged == nil {
		dealChanged = map[string]any{}
	}
	dealChanged["products"] = allocationImage(allocated)
	return dealChanged
}
