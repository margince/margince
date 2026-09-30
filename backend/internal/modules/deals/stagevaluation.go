// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type stageValuation struct {
	amount   *int64
	currency *string
	rate     pgtype.Numeric
	on       *time.Time
	reason   string
}

func (s *Store) stageValuation(ctx context.Context, tx pgx.Tx, in stageHistoryInput) (stageValuation, error) {
	out := stageValuation{reason: "unpriced"}
	if in.Amount == nil || in.Currency == nil {
		return out, nil
	}
	base, err := s.installation.BaseCurrency(ctx, tx)
	if err != nil {
		return out, err
	}
	out.currency = &base
	rates := NewFXRates(base, s.clock())
	priced, err := PriceAll(ctx, tx, rates, []CurrencyAmount{{Minor: *in.Amount, Currency: *in.Currency}})
	if err != nil {
		return out, err
	}
	if !priced[0].Priced {
		out.reason = "valuation_unavailable"
		return out, nil
	}
	rate, found, err := rates.For(ctx, tx, *in.Currency)
	if err != nil {
		return out, err
	}
	if !found {
		out.reason = "fx_missing"
		return out, nil
	}
	out.amount = &priced[0].Minor
	out.rate = rate.Rate
	out.on = &rate.On
	out.reason = "frozen_at_transition"
	return out, nil
}
