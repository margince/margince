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

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
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

// frozenBefore is a deal's currently frozen pair in the shape the patch records
// it. The rate is already a decimal string; the date sheds the contract's Date
// wrapper, which is what Patch.SetDate takes on both sides.
//
// The frozen base AMOUNT is not here: amount_minor_base is an internal column
// and carries no field on the contract, so its pre-image is read from the row
// by frozenBaseBefore rather than invented from a shape that does not hold it.
func frozenBefore(deal crmcontracts.Deal) (rate *string, rateDate *time.Time) {
	return deal.FxRateToBase, storekit.PlainDate(deal.FxRateDate)
}

// frozenBaseBefore reads the frozen base amount a deal currently holds, for the
// patch's pre-image. A separate read because the column is internal: it is not
// on crmcontracts.Deal, and a writer that recorded nil instead would put "there
// was no converted amount" in the audit diff of every re-price and reopen.
func frozenBaseBefore(ctx context.Context, tx pgx.Tx, id openapi_types.UUID) (*int64, error) {
	var base *int64
	if err := tx.QueryRow(ctx,
		`SELECT amount_minor_base FROM deal WHERE id = $1`, id).Scan(&base); err != nil {
		return nil, fmt.Errorf("read frozen base amount: %w", err)
	}
	return base, nil
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
// The caller passes what the row held, because two of the three writers reach
// here for a deal that ALREADY carries a frozen rate — deal_closed_fx requires
// one on any closed deal with an amount — and re-pricing replaces it. Recording
// the pre-image as nil would put "there was no rate" in the audit diff of every
// re-price, which is the one row a reversal reads to restore the old one.
//
// The error comes back unwrapped: a caller closing a deal, re-pricing one and
// accepting an offer each tell an operator more by naming which than a shared
// sentence could.
func (s *Store) freezeBaseRate(ctx context.Context, tx pgx.Tx, p *storekit.Patch,
	dealID openapi_types.UUID, currency string, amountMinor int64, asOf time.Time,
	rateBefore *string, rateDateBefore *time.Time,
) error {
	baseBefore, err := frozenBaseBefore(ctx, tx, dealID)
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
	p.Set(fxRateColumn, rateBefore, rate)
	p.SetDate(fxRateDateColumn, rateDateBefore, &rateDate)
	p.Set(baseAmountColumn, baseBefore, converted)
	return nil
}

// freezeFx resolves the frozen currency→base conversion for a closed
// deal: the latest fx_rate on or before asOf. Used at close (asOf = now)
// and when a closed deal is re-priced (asOf = its close date), so the
// frozen rate always reflects the deal's close, never the edit.
func (s *Store) freezeFx(ctx context.Context, tx pgx.Tx,
	base, currency string, asOf time.Time,
) (string, time.Time, error) {
	asOfDate := asOf.UTC().Truncate(24 * time.Hour)
	if currency == base {
		return "1", asOfDate, nil
	}
	var err error
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
