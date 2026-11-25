// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// A deal's value is the figure without tax. Accepting an offer used to write the
// offer's gross onto the deal; this corrects the deals that took one.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// grossPricedDealsSQL selects the deals whose amount is the gross of their
// latest accepted offer. Its accept row must say the accept wrote that figure.
// A figure someone typed by hand leaves no such row.
// A deal in lines mode takes its value from its lines.
const grossPricedDealsSQL = `
	FROM deal d
	JOIN LATERAL (
	    SELECT o.id, o.net_minor, o.gross_minor
	      FROM offer o
	     WHERE o.deal_id = d.id AND o.status = 'accepted' AND o.currency = d.currency
	     ORDER BY o.accepted_at DESC, o.id DESC
	     LIMIT 1
	) o ON true
	WHERE d.commercial_mode <> 'lines'
	  AND o.gross_minor <> o.net_minor
	  AND d.amount_minor = o.gross_minor
	  AND EXISTS (
	      SELECT 1 FROM audit_log a
	       WHERE a.entity_type = 'offer' AND a.entity_id = o.id AND a.action = 'update'
	         AND a.after ->> 'status' = 'accepted'
	         AND a.after -> 'deal_amount_minor' = to_jsonb(o.gross_minor))`

// NetCorrection is one deal re-priced from its accepted offer's gross to the net.
type NetCorrection struct {
	Deal       ids.DealID
	Name       string
	Owner      *ids.UUID
	Currency   string
	FromMinor  int64
	ToMinor    int64
	offerID    ids.OfferID
	grossMinor int64
}

// NetCorrectionPage is one inspected page: the deals looked at, and the ones
// that were corrected.
type NetCorrectionPage struct {
	Scanned   []ids.UUID
	Corrected []NetCorrection
}

// RepriceFromAcceptedOfferNetTx corrects one page of gross-priced deals, after
// the given deal id. Each goes through the deal patch seam, so the forecast move,
// the audit row and deal.updated are written as for the accept itself.
// A closed deal is re-frozen on the net.
// ARR was derived from net and is not touched.
// Commission entries keep the basis they recorded.
func (s *Store) RepriceFromAcceptedOfferNetTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (NetCorrectionPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return NetCorrectionPage{}, err
	}
	rows, err := tx.Query(ctx, `
		SELECT d.id, d.name, d.owner_id, d.currency, d.amount_minor, o.id, o.net_minor, o.gross_minor `+
		grossPricedDealsSQL+` AND d.id > $1
		ORDER BY d.id LIMIT $2`, after, limit)
	if err != nil {
		return NetCorrectionPage{}, fmt.Errorf("deals: listing gross-priced deals: %w", err)
	}
	candidates, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (NetCorrection, error) {
		var c NetCorrection
		err := row.Scan(&c.Deal, &c.Name, &c.Owner, &c.Currency, &c.FromMinor, &c.offerID, &c.ToMinor, &c.grossMinor)
		return c, err
	})
	if err != nil {
		return NetCorrectionPage{}, fmt.Errorf("deals: reading gross-priced deals: %w", err)
	}
	var page NetCorrectionPage
	for _, c := range candidates {
		page.Scanned = append(page.Scanned, c.Deal.UUID)
		done, err := s.repriceOneToNet(ctx, tx, &c)
		if err != nil {
			return NetCorrectionPage{}, err
		}
		if done {
			page.Corrected = append(page.Corrected, c)
		}
	}
	return page, nil
}

// repriceOneToNet re-reads the deal under its lock, because someone may have
// retyped the amount since the page was listed. It answers whether it wrote.
func (s *Store) repriceOneToNet(ctx context.Context, tx pgx.Tx, c *NetCorrection) (bool, error) {
	var status string
	var closedAt *time.Time
	var amount *int64
	lock, err := storekit.LockRowReading(ctx, tx, dealTable, c.Deal.UUID, storekit.IncludeArchived,
		"status, closed_at, amount_minor", &status, &closedAt, &amount)
	if err != nil {
		return false, fmt.Errorf("deals: locking deal %s for its net value: %w", c.Deal, err)
	}
	if amount == nil || *amount != c.grossMinor {
		return false, nil
	}
	p := storekit.NewPatch()
	p.Set(amountField, amount, c.ToMinor)
	if DealStatus(status) != DealOpen {
		if err := s.freezeBaseRate(ctx, tx, p, openapi_types.UUID(c.Deal.UUID), c.Currency, c.ToMinor, *closedAt); err != nil {
			return false, fmt.Errorf("deals: re-freezing closed deal %s on its net value: %w", c.Deal, err)
		}
	}
	if err := applyDealPatchLocked(ctx, tx, p, lock); err != nil {
		return false, fmt.Errorf("deals: pricing deal %s at its offer's net: %w", c.Deal, err)
	}
	if err := auditDealRepricing(ctx, tx, c.Deal, c.offerID, p.Before(), p.After()); err != nil {
		return false, err
	}
	return true, nil
}

// EstimateGrossPricedDeals counts, up to limit+1, the deals a correction
// would re-price.
func EstimateGrossPricedDeals(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, limit int64,
) (int64, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return 0, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 `+grossPricedDealsSQL+` LIMIT $1) gross`, limit+1).Scan(&count)
	return count, err
}
