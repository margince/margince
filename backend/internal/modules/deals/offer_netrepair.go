// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// A deal's value is the figure without tax. Accepting an offer used to write the
// offer's gross onto the deal; this corrects the deals that took one.

import (
	"context"
	"errors"
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
// latest accepted offer, written by that offer's accept and by nobody since.
// The anchor is the deal audit row the accept wrote for the re-price. A later
// deal audit row that changed the amount means someone retyped it, even to the
// same figure. A deal in lines mode takes its value from its lines.
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
	       WHERE a.entity_type = 'deal' AND a.entity_id = d.id AND a.action = 'update'
	         AND a.evidence ->> 'offer_id' = o.id::text
	         AND a.after -> 'amount_minor' = to_jsonb(o.gross_minor)
	         AND NOT EXISTS (
	             SELECT 1 FROM audit_log later
	              WHERE later.entity_type = 'deal' AND later.entity_id = d.id
	                AND later.after ? 'amount_minor'
	                AND (later.occurred_at, later.id) > (a.occurred_at, a.id)))`

// NetCorrection is one deal re-priced from its accepted offer's gross to the net.
// Every field is read under the deal's lock, so the owner and name are the
// ones the deal has when it is corrected.
type NetCorrection struct {
	Deal      ids.DealID
	Name      string
	Owner     *ids.UUID
	Currency  string
	FromMinor int64
	ToMinor   int64
	offerID   ids.OfferID
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
	rows, err := tx.Query(ctx, `SELECT d.id `+grossPricedDealsSQL+` AND d.id > $1 ORDER BY d.id LIMIT $2`, after, limit)
	if err != nil {
		return NetCorrectionPage{}, fmt.Errorf("deals: listing gross-priced deals: %w", err)
	}
	candidates, err := pgx.CollectRows(rows, pgx.RowTo[ids.DealID])
	if err != nil {
		return NetCorrectionPage{}, fmt.Errorf("deals: reading gross-priced deals: %w", err)
	}
	var page NetCorrectionPage
	for _, id := range candidates {
		page.Scanned = append(page.Scanned, id.UUID)
		corrected, done, err := s.repriceOneToNet(ctx, tx, id)
		if err != nil {
			return NetCorrectionPage{}, err
		}
		if done {
			page.Corrected = append(page.Corrected, corrected)
		}
	}
	return page, nil
}

// repriceOneToNet locks the deal, then reads everything it acts on under that
// lock. Someone may have reassigned, retyped or re-priced the deal since the
// page was listed. The rule is asked again about the deal as it is now.
// It answers whether it wrote.
func (s *Store) repriceOneToNet(ctx context.Context, tx pgx.Tx, id ids.DealID) (NetCorrection, bool, error) {
	lock, err := storekit.LockRow(ctx, tx, dealTable, id.UUID, storekit.IncludeArchived)
	if err != nil {
		return NetCorrection{}, false, fmt.Errorf("deals: locking deal %s for its net value: %w", id, err)
	}
	if err := auth.EnsureWritableLive(ctx, tx, dealTable, id.UUID); err != nil {
		return NetCorrection{}, false, err
	}
	c := NetCorrection{Deal: id}
	var status string
	var closedAt *time.Time
	var amount *int64
	var gross int64
	err = tx.QueryRow(ctx, `
		SELECT d.name, d.owner_id, d.currency, d.status, d.closed_at, d.amount_minor, o.id, o.net_minor, o.gross_minor `+
		grossPricedDealsSQL+` AND d.id = $1`, id).
		Scan(&c.Name, &c.Owner, &c.Currency, &status, &closedAt, &amount, &c.offerID, &c.ToMinor, &gross)
	if errors.Is(err, pgx.ErrNoRows) {
		return NetCorrection{}, false, nil
	}
	if err != nil {
		return NetCorrection{}, false, fmt.Errorf("deals: re-reading deal %s under its lock: %w", id, err)
	}
	c.FromMinor = gross
	p := storekit.NewPatch()
	p.Set(amountField, amount, c.ToMinor)
	if DealStatus(status) != DealOpen {
		if err := s.freezeBaseRate(ctx, tx, p, openapi_types.UUID(id.UUID), c.Currency, c.ToMinor, *closedAt); err != nil {
			return NetCorrection{}, false, fmt.Errorf("deals: re-freezing closed deal %s on its net value: %w", id, err)
		}
	}
	if err := applyDealPatchLocked(ctx, tx, p, lock); err != nil {
		return NetCorrection{}, false, fmt.Errorf("deals: pricing deal %s at its offer's net: %w", id, err)
	}
	if err := auditDealRepricing(ctx, tx, id, c.offerID, p.Before(), p.After()); err != nil {
		return NetCorrection{}, false, err
	}
	return c, true, nil
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
