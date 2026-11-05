// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Freezing a deal's conversion at close.
//
// Three writers reach this: a stage advance that closes the deal, a re-price of
// a closed one, and an accepted offer. They disagree only about the day the
// rate is taken on, so the resolution lives here once and each passes its own
// asOf.
//
// What is frozen is a rate AND the amount it converts to. Storing only the rate
// would leave every reader to apply both currencies' minor-unit scales, and the
// column spent its first release as a generated expression that could not reach
// them — round(amount_minor x rate), which is a hundredth of the truth for a
// zero-decimal currency against a two-decimal base.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// MissingFxRateError maps to 422: closing a foreign-currency deal needs a
// same-day-or-earlier fx_rate row to freeze.
type MissingFxRateError struct{ From, To string }

func (e *MissingFxRateError) Error() string {
	return "no fx_rate from " + e.From + " to " + e.To + " to freeze at close"
}

// MessageFault names the condition and no field: the spec's hard-fail
// (formulas §6.1) fires because the workspace holds no rate for this currency
// pair — server-side data, not an argument. Naming fx_rate_to_base would tell
// an agent to correct an input it never sent and cannot supply.
func (e *MissingFxRateError) MessageFault() (code, message string) {
	return "fx_rate_unavailable", e.Error() + " — an admin must load the rate for this currency pair before this close can succeed"
}

// FreezeRateAt resolves what a currency converts at, as of a day, against the
// installation's own base — the same reading a closing deal freezes.
//
// Exported for a caller outside this module that has to freeze the same
// number: a contract, at activation. It could read fx_rate itself — the table
// is deals', and the ownership gate binds writes rather than reads — and then
// there would be two spellings of "the latest rate on or before this day", free
// to disagree about the boundary, the same-currency shortcut, or what a missing
// rate means. One of them would be corrected.
//
// Handed over as a function rather than as this store, so the caller takes a
// seam and not a module (the shape counters' BaseCurrencyFunc already uses).
func (s *Store) FreezeRateAt(ctx context.Context, tx pgx.Tx, currency string, asOf time.Time) (string, time.Time, error) {
	base, err := s.installation.BaseCurrency(ctx, tx)
	if err != nil {
		return "", time.Time{}, err
	}
	return s.freezeFx(ctx, tx, base, currency, asOf)
}

type frozenConversion struct {
	Rate   *string
	Date   *time.Time
	Amount *int64
}

// Frozen columns are internal; audit pre-images cannot come from the public deal.
func frozenBefore(ctx context.Context, tx pgx.Tx, id openapi_types.UUID) (frozenConversion, error) {
	var before frozenConversion
	args := []any{id}
	err := tx.QueryRow(ctx, storekit.SQLf(`SELECT fx_rate_to_base::text, fx_rate_date, amount_minor_base FROM deal WHERE id = $%d`, len(args)), args...).Scan(&before.Rate, &before.Date, &before.Amount)
	return before, err
}

func clearFrozenConversion(ctx context.Context, tx pgx.Tx, p *storekit.Patch, id openapi_types.UUID) error {
	before, err := frozenBefore(ctx, tx, id)
	if err != nil {
		return err
	}
	p.Set(fxRateColumn, before.Rate, nil)
	p.SetDate(fxRateDateColumn, before.Date, nil)
	if before.Amount != nil {
		p.Set(baseAmountColumn, before.Amount, nil)
	}
	return nil
}

// freezeBaseRate stamps a frozen conversion onto a patch: FreezeRateAt above
// decides WHAT the rate is, and this decides which two columns carry it.
//
// It is built on that seam rather than resolving the base currency itself,
// because "the latest rate on or before this day" is one question and two
// spellings of it would be free to disagree about the day boundary, the
// same-currency shortcut, or what a missing rate means.
//
// A closed deal must carry a rate for the currency it is priced in. Re-pricing
// one into a DIFFERENT currency and leaving the old rate would convert the wrong
// pair, which corrupts the base-currency roll-up silently; and a deal closed
// with no amount has no frozen rate at all, so the write that first prices it
// trips deal_closed_fx unless it freezes one in the same statement.
//
// asOf is the caller's, because it is the only thing the three writers disagree
// about: a stage advance closing the deal freezes as of the close, while the two
// re-pricing doors freeze as of the ORIGINAL close date, for the reason freezeFx
// states below.
//
// The error comes back unwrapped: a caller closing a deal, re-pricing one and
// accepting an offer each tell an operator more by naming which than a shared
// sentence could.
func (s *Store) freezeBaseRate(ctx context.Context, tx pgx.Tx, p *storekit.Patch,
	dealID openapi_types.UUID, currency string, amountMinor int64, asOf time.Time,
) error {
	before, err := frozenBefore(ctx, tx, dealID)
	if err != nil {
		return err
	}
	rate, rateDate, err := s.FreezeRateAt(ctx, tx, currency, asOf)
	if err != nil {
		return err
	}
	base, err := s.installation.BaseCurrency(ctx, tx)
	if err != nil {
		return err
	}
	// The converted amount is frozen alongside the rate rather than derived
	// from it later. A reader deriving it would have to apply both minor-unit
	// scales itself, and the column spent its first release as a GENERATED
	// expression that could not reach them — round(amount_minor x rate) alone,
	// which is a hundred times short for a zero-decimal currency against a
	// two-decimal base. ConvertToBase is the one place that arithmetic lives.
	var parsed pgtype.Numeric
	if err := parsed.Scan(rate); err != nil {
		return fmt.Errorf("frozen rate %q is not a number: %w", rate, err)
	}
	converted, err := ConvertToBase(amountMinor, parsed, currency, base)
	if err != nil {
		return err
	}
	p.Set(fxRateColumn, before.Rate, rate)
	p.SetDate(fxRateDateColumn, before.Date, &rateDate)
	p.Set(baseAmountColumn, before.Amount, converted)
	return nil
}

// freezeFx resolves the frozen currency→base conversion for a closed
// deal: the latest fx_rate on or before asOf. Used at close (asOf = now)
// and when a closed deal is re-priced (asOf = its close instant), so the
// frozen rate always reflects the deal's close, never the edit.
//
// asOf is an INSTANT — a close moment or the clock — and the calendar day it
// freezes against is that instant read in the installation's zone, not UTC: an
// installation ahead of UTC closing in its local morning would otherwise freeze
// the previous day's rate, and a frozen rate is a business and audit fact that
// is never recomputed. The day is derived HERE, so every caller passes a raw
// instant rather than a second spelling of "which day".
func (s *Store) freezeFx(ctx context.Context, tx pgx.Tx,
	base, currency string, asOf time.Time,
) (string, time.Time, error) {
	tzName, err := s.installation.Timezone(ctx, tx)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("resolve the installation's timezone: %w", err)
	}
	loc, err := installationZone(tzName)
	if err != nil {
		return "", time.Time{}, err
	}
	asOfDate := storekit.WorkspaceDay(asOf, loc)
	if currency == base {
		return "1", asOfDate, nil
	}
	var rate string
	err = tx.QueryRow(ctx,
		`SELECT rate::text FROM fx_rate
		 WHERE from_currency = $1 AND to_currency = $2 AND rate_date <= $3
		 ORDER BY rate_date DESC LIMIT 1`,
		currency, base, asOfDate).Scan(&rate)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", time.Time{}, &MissingFxRateError{From: currency, To: base}
	}
	if err != nil {
		return "", time.Time{}, err
	}
	return rate, asOfDate, nil
}
