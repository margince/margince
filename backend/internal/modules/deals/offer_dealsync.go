// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// An accepted offer's figures reaching the deal it prices.
//
// Its own file because it is the one place an OFFER writes DEAL money: the
// accept path decides that the offer is final, and this decides what that means
// for the deal's amount, currency and — when the deal is already closed — the
// conversion frozen on it.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// syncDealAmountFromOffer writes the accepted gross onto the deal. A
// still-open deal takes the amount as-is; a deal that already closed carries the
// re-freeze freezeBaseRate states, which is the invariant applyMoneyInvariants
// enforces on direct deal edits: re-pricing a deal is one rule whichever door
// the price arrives through.
//
// The write goes through the deal patch seam rather than its own UPDATE, so the
// deal's forecast move is recorded with it. Pricing a deal from its accepted
// offer changes no stage, which is exactly the move deal_stage_history cannot
// carry.
//
// It writes the money only where the deal does not already hold it. An accept
// that prices the deal at what it was already priced at moved no forecast, and a
// history row saying otherwise is a move a reconstruction would have to explain.
//
// It returns the deal columns the sync actually wrote — nothing when it wrote
// nothing — so the caller's paired deal.updated reports the complete delta: on a
// closed deal that includes the re-frozen fx_rate_to_base/fx_rate_date, not just
// amount_minor/currency.
func (s *Store) syncDealAmountFromOffer(ctx context.Context, tx pgx.Tx,
	dealID ids.DealID, offer crmcontracts.Offer,
) (map[string]any, error) {
	if offer.GrossMinor == nil {
		// The totals engine derives a gross for every line and send refuses an
		// offer with none, so a sent offer always carries one. Refusing rather
		// than writing NULL keeps that reasoning falsifiable: a null amount
		// beside the currency below would trip deal_amount_currency_pair, and
		// the row would fail with nothing naming the offer that did it.
		return nil, fmt.Errorf("accepted offer %s carries no gross to price deal %s with", offer.Id, dealID)
	}
	// The row lock makes the status read and the amount write below one
	// race-free unit. IncludeArchived preserves the read below, which
	// follows the deal row regardless of archived state.
	lock, err := storekit.LockRow(ctx, tx, dealTable, dealID.UUID, storekit.IncludeArchived)
	if err != nil {
		return nil, fmt.Errorf("lock deal for amount sync: %w", err)
	}
	var status string
	var closedAt *time.Time
	var amountBefore *int64
	var currencyBefore *string
	var rateBefore *string
	var rateDateBefore *time.Time
	// The frozen pair is read for its pre-image, not for a decision: re-pricing a
	// closed deal replaces a rate it already carries, and the audit diff has to
	// say which one.
	if err := tx.QueryRow(ctx,
		`SELECT status, closed_at, amount_minor, currency, fx_rate_to_base::text, fx_rate_date
		   FROM deal WHERE id = $1`,
		dealID).Scan(&status, &closedAt, &amountBefore, &currencyBefore,
		&rateBefore, &rateDateBefore); err != nil {
		return nil, fmt.Errorf("read deal for amount sync: %w", err)
	}

	// The columns are nullable and the offer's figures are not, so each half is
	// compared on its own terms. An unpriced deal and a priced one are different
	// forecasts; so are two different prices; and re-pricing at the figure the
	// deal already carries is neither.
	p := storekit.NewPatch()
	if amountBefore == nil || *amountBefore != *offer.GrossMinor {
		p.Set(amountField, amountBefore, *offer.GrossMinor)
	}
	if currencyBefore == nil || *currencyBefore != offer.Currency {
		p.Set(currencyField, currencyBefore, offer.Currency)
	}
	if p.Empty() {
		// The deal already holds this offer's figures: no write, so no history
		// row, and an empty delta for the caller's paired event to find nothing in.
		return p.After(), nil
	}
	if DealStatus(status) != DealOpen {
		// deal_closed_at guarantees closedAt on a non-open row.
		if err := s.freezeBaseRate(ctx, tx, p, openapi_types.UUID(dealID.UUID), offer.Currency,
			*offer.GrossMinor, *closedAt, rateBefore, rateDateBefore); err != nil {
			return nil, fmt.Errorf("re-freeze fx for closed deal on accept: %w", err)
		}
	}
	if err := applyDealPatchLocked(ctx, tx, p, lock); err != nil {
		return nil, fmt.Errorf("sync deal amount from offer: %w", err)
	}
	return p.After(), nil
}
