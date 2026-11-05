// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The currency arithmetic every finance store is wired with.
//
// fx_rate and the conversion belong to deals, and a mirrored invoice asks what
// a deal asks. Which rate is newest on or before a day? What does the base
// currency convert at? What do both minor-unit scales make of an amount?
// Finance takes the answers as a seam, so each rule has one spelling. Every
// finance store is built through FinanceRates, so no two are wired apart.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/finance"
	"github.com/margince/margince/backend/internal/modules/identity"
)

// FinanceRates answers the installation's base currency, the rate in force on
// an invoice's issue date, and the conversion, from the modules that own them.
func FinanceRates() finance.Rates {
	return finance.Rates{
		BaseCurrency: identity.BaseCurrencyOf,
		RateOn:       financeRateOn,
		ToBase:       deals.ConvertToBase,
	}
}

// financeRateOn reads the day as a UTC calendar date, which is how fx_rate
// dates its rows and how an invoice states its issue date. The deals store's
// freeze reads an instant in the installation zone instead, and would move an
// issue date a day west of UTC.
func financeRateOn(
	ctx context.Context, tx pgx.Tx, currency, base string, day time.Time,
) (finance.Rate, bool, error) {
	rate, found, err := deals.NewFXRates(base, day).For(ctx, tx, currency)
	return finance.Rate{Value: rate.Rate, On: rate.On}, found, err
}
