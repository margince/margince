// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

// What rate a mirrored invoice converts at, and how its amounts reach the base
// currency.
//
// The rate sheet and the one conversion belong to deals, so compose hands them
// in as Rates and finance spells neither rule itself. An invoice freezes the
// rate in force on its issue date (FIN-PARAM-7/DM-FX-4). A day with no rate
// leaves the row unconverted, and the formulas refuse the total (FIN-AC-6).

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Rate is one stored conversion rate and the day it is dated. The schema's
// CHECK wants both or neither, so they travel as one value.
type Rate struct {
	Value pgtype.Numeric
	On    time.Time
}

// RateOnFunc answers the rate that converts currency into base on day, with
// found false when the sheet holds none. A currency equal to base answers the
// identity, dated day.
type RateOnFunc func(ctx context.Context, tx pgx.Tx, currency, base string, day time.Time) (rate Rate, found bool, err error)

// ToBaseFunc converts an amount in from's minor units into base's minor units.
// The rate counts major units, so both currencies' scales apply.
type ToBaseFunc func(amountMinor int64, rate pgtype.Numeric, from, base string) (int64, error)

// Rates is the currency arithmetic the mirror borrows from the modules that own
// it. Every field is required.
type Rates struct {
	BaseCurrency BaseCurrencyFunc
	RateOn       RateOnFunc
	ToBase       ToBaseFunc
}

// rateFor is the rate an invoice freezes at, or found false when none is in
// force on its issue date. An unknown base has no rate, because converting
// into it would invent a figure.
func (s *Store) rateFor(
	ctx context.Context, tx pgx.Tx, inv SourceInvoice, base string,
) (Rate, bool, error) {
	if base == "" {
		return Rate{}, false, nil
	}
	return s.rates.RateOn(ctx, tx, strings.ToUpper(inv.Currency), base, inv.IssuedOn)
}
