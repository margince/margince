// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The Worklist's money, priced by the one FX engine.
//
// The ranked queue compares expected revenue between deals, and that comparison
// is honest only in the installation's base currency. The conversion itself —
// the direction, the as-of cutoff, newest-wins, the multiply-and-round — is
// deals.FXRates + deals.ConvertToBase, the same engine the company page and the
// hierarchy rollup price with, bound here as a seam so the queue cannot grow a
// second spelling of it.
//
// The missing-rate policy is the caller's (fxconvert.go states why): this
// surface leaves an unpriceable deal UNPRICED, so it ranks as a deal whose
// value nobody recorded rather than by a number in the wrong units, and the
// row still shows its own amount in its own currency.

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
		rates := deals.NewFXRates(base, asOf)
		for i, amount := range amounts {
			rate, found, err := rates.For(ctx, tx, amount.Currency)
			if err != nil {
				return err
			}
			if !found {
				continue
			}
			converted, err := deals.ConvertToBase(amount.Minor, rate.Rate)
			if err != nil {
				// UNPRICED, not refused — a decision, not a swallow. An amount
				// whose converted value does not fit money's range is one deal
				// the ordering cannot weigh, and org360's open-pipeline read
				// gives the same deal the same answer; one implausible amount
				// must not take the whole queue offline.
				continue
			}
			out[i] = &converted
		}
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	return out, base, nil
}
