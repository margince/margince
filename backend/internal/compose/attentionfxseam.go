// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The Worklist's money, priced by the one FX engine.
//
// The ranked queue compares expected revenue between deals, and that comparison
// is honest only in the installation's base currency. Nothing about HOW is
// decided here: deals.PriceAll runs the loop — which rate, what a missing one
// means, and the two currencies' minor-unit scales — and this file is only the
// wiring that hands it the installation's base and this read's amounts.
//
// deals.PriceAll's leave-it-unpriced policy is the one this surface wants: an
// unpriceable deal ranks as a deal whose value nobody recorded rather than by a
// number in the wrong units, and the row still shows its own amount in its own
// currency.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
)

// AttentionBaseMoney prices one read's amounts against the stored rates — the
// seam the Worklist is wired with. Exported the way OrgHierarchyRollup is: the
// integration lane proves it against real rates, because the cutoff,
// newest-wins and the identity shortcut are SQL a unit test cannot fail.
type AttentionBaseMoney struct{ Pool *pgxpool.Pool }

// ToBase answers each amount in the installation's base currency, nil where
// the estate cannot price it — attention.BaseMoney states the full contract.
func (m AttentionBaseMoney) ToBase(
	ctx context.Context, asOf time.Time, amounts []attention.CurrencyAmount,
) ([]*int64, string, error) {
	out := make([]*int64, len(amounts))
	var base string
	err := database.WithWorkspaceTx(ctx, m.Pool, func(tx pgx.Tx) error {
		var err error
		base, err = identity.BaseCurrencyOf(ctx, tx)
		if err != nil {
			return err
		}
		figures := make([]deals.CurrencyAmount, len(amounts))
		for i, amount := range amounts {
			figures[i] = deals.CurrencyAmount{Minor: amount.Minor, Currency: amount.Currency}
		}
		priced, err := deals.PriceAll(ctx, tx, deals.NewFXRates(base, asOf), figures)
		if err != nil {
			return err
		}
		// The rate DATE goes no further: the queue states an order, and a row
		// that named the day its comparison was priced at would be answering a
		// question nobody asked of a ranking.
		for i, figure := range priced {
			if figure.Priced {
				minor := figure.Minor
				out[i] = &minor
			}
		}
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	return out, base, nil
}
