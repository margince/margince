// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// heldFxRate is the day's current row when it already holds rate. The rates
// are compared as numbers, so another spelling of the same rate is no change.
func heldFxRate(ctx context.Context, tx pgx.Tx, from, base string, effDate time.Time, rate string) (FxRateRow, bool, error) {
	var out FxRateRow
	err := tx.QueryRow(ctx, `
		SELECT from_currency, to_currency, rate::text, rate_date FROM fx_rate
		WHERE from_currency = $1 AND to_currency = $2 AND rate_date = $3
		  AND `+RateVersionSQL("", "")+` AND rate = $4::numeric`,
		from, base, effDate, rate).Scan(&out.FromCurrency, &out.ToCurrency, &out.Rate, &out.RateDate)
	if errors.Is(err, pgx.ErrNoRows) {
		return FxRateRow{}, false, nil
	}
	if err != nil {
		return FxRateRow{}, false, fmt.Errorf("compare the day's fx_rate: %w", err)
	}
	return out, true, nil
}

// refuseRewritingHistory keeps the sheet append-forward with one exception: a
// past date strictly earlier than every stored rate for the currency. Such a
// row only prices days that had no rate, so no figure already computed moves.
// The caller holds the currency's write-identity lock, so the earliest date
// cannot change between this read and the write.
func refuseRewritingHistory(ctx context.Context, tx pgx.Tx, from string, effDate, today time.Time) error {
	if !effDate.Before(today) {
		return nil
	}
	var earliest *time.Time
	if err := tx.QueryRow(ctx,
		`SELECT min(rate_date) FROM fx_rate /* every version */ WHERE from_currency = $1`, from).Scan(&earliest); err != nil {
		return fmt.Errorf("read the earliest fx_rate for %s: %w", from, err)
	}
	if earliest == nil || effDate.Before(*earliest) {
		return nil
	}
	return fxInvalid("effective_date", "fx_rate_past", fmt.Sprintf(
		"effective_date cannot be in the past unless it is before the earliest %s rate (%s)",
		from, earliest.Format(time.DateOnly)))
}
