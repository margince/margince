// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The deal list's commercial dials: when a deal closes, what it is worth, what
// it is priced in, and where its company is. Each reads the expression the
// list's own column and sort read, so a filter and a sort never disagree.

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/fieldmask"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// The wire names of the commercial dials, beside the others in listfilters.go.
const (
	filterCloseFrom = "close_from"
	filterCloseTo   = "close_to"
	filterValueMin  = "value_min"
	filterValueMax  = "value_max"
	filterCurrency  = "currency"
	filterCountry   = "country"
)

// RangeFilters are the commercial dials of a deal list. Every bound is
// inclusive, and a deal with nothing to compare is outside every range.
type RangeFilters struct {
	// CloseFrom and CloseTo bound the close day: the day a closed deal closed
	// in the installation's zone, or an open deal's expected close date.
	CloseFrom *time.Time
	CloseTo   *time.Time
	// ValueMin and ValueMax bound the value in minor units of the base
	// currency, as the Value sort reads it.
	ValueMin *int64
	ValueMax *int64
	// Currency is the ISO-4217 code the deal is priced in, and Country the
	// ISO-3166 alpha-2 code on its company's address. Either case matches.
	Currency *string
	Country  *string
	// valuation is the base currency, rate day and zone, read by the store
	// itself when readsValuation says a dial needs them.
	valuation dealValuation
}

// widenMinor turns a seam operand into the field's own width.
func widenMinor(v *int) *int64 {
	minor := int64(*v)
	return &minor
}

// readsValuation reports whether the dials need the base currency, the rate
// day or the zone, which the list then reads in its own transaction.
func (f RangeFilters) readsValuation() bool {
	return f.CloseFrom != nil || f.CloseTo != nil || f.ValueMin != nil || f.ValueMax != nil
}

// appendRangeFilters adds the commercial dials to a deal list's clauses.
func appendRangeFilters(ctx context.Context, where []string, f RangeFilters, arg func(any) int) ([]string, error) {
	if err := refuseNegativeValues(f); err != nil {
		return nil, err
	}
	if err := refuseInvertedRanges(f); err != nil {
		return nil, err
	}
	if f.CloseFrom != nil || f.CloseTo != nil {
		day := closeDaySQL(arg(f.valuation.zone))
		where = appendBounds(where, day, "date", calendarDay(f.CloseFrom), calendarDay(f.CloseTo), arg)
	}
	if f.ValueMin != nil || f.ValueMax != nil {
		if err := refuseMaskedValue(ctx, f); err != nil {
			return nil, err
		}
		where = appendBounds(where, valueInBaseSQL(f.valuation, arg), "bigint", f.ValueMin, f.ValueMax, arg)
	}
	if f.Currency != nil {
		currency, err := codeOperand(*f.Currency, filterCurrency, values.ValidCurrency, "a three-letter ISO-4217 code, such as EUR")
		if err != nil {
			return nil, err
		}
		if err := auth.RefuseMaskedFilter(ctx, maskObject, filterCurrency, fieldmask.DealCurrency); err != nil {
			return nil, err
		}
		where = append(where, storekit.SQLf("currency = $%d", arg(currency)))
	}
	if f.Country != nil {
		country, err := codeOperand(*f.Country, filterCountry, countryShape.MatchString, "a two-letter ISO 3166-1 code, such as DE")
		if err != nil {
			return nil, err
		}
		clause, err := countryFilterClause(ctx, country, arg)
		if err != nil {
			return nil, err
		}
		where = append(where, clause)
	}
	return where, nil
}

// countryShape is the alpha-2 shape the company address column stores.
var countryShape = regexp.MustCompile(`^[A-Z]{2}$`)

// codeOperand upper-cases a currency or country dial and refuses one of the
// wrong shape. It could match no deal, and the empty page would read as "none".
func codeOperand(raw, field string, shaped func(string) bool, shape string) (string, error) {
	code := strings.Map(upperASCII, raw)
	if shaped(code) {
		return code, nil
	}
	return "", &values.ParseError{Field: field, Code: field + "_malformed", Message: field + " must be " + shape}
}

// upperASCII folds an ASCII letter alone. strings.ToUpper also folds some
// non-ASCII letters into ASCII ones, which would pass a non-code as a code.
func upperASCII(r rune) rune {
	if 'a' <= r && r <= 'z' {
		return r - ('a' - 'A')
	}
	return r
}

// appendBounds adds `expr >= low` and `expr <= high` for each bound given. A
// NULL expression fails both, which keeps a deal with no figure out.
func appendBounds[T string | int64](where []string, expr, cast string, low, high *T, arg func(any) int) []string {
	if low != nil {
		where = append(where, storekit.SQLf("(%s) >= $%d::%s", expr, arg(*low), cast))
	}
	if high != nil {
		where = append(where, storekit.SQLf("(%s) <= $%d::%s", expr, arg(*high), cast))
	}
	return where
}

// calendarDay spells a bound the way the contract's dates are spelled.
func calendarDay(day *time.Time) *string {
	if day == nil {
		return nil
	}
	iso := day.Format(time.DateOnly)
	return &iso
}

// refuseNegativeValues refuses a value bound below zero, as the contract's
// minimum does. It holds the agent's path, which the HTTP check never sees.
func refuseNegativeValues(f RangeFilters) error {
	for _, bound := range []struct {
		field string
		value *int64
	}{{filterValueMin, f.ValueMin}, {filterValueMax, f.ValueMax}} {
		if bound.value != nil && *bound.value < 0 {
			return &values.ParseError{
				Field: bound.field, Code: "out_of_range",
				Message: bound.field + " must be at least 0",
			}
		}
	}
	return nil
}

// refuseInvertedRanges refuses a lower bound above its upper one. Such a range
// selects nothing, and an empty page would read as no deals rather than a typo.
func refuseInvertedRanges(f RangeFilters) error {
	if f.CloseFrom != nil && f.CloseTo != nil && f.CloseFrom.After(*f.CloseTo) {
		return &values.ParseError{
			Field: filterCloseFrom, Code: "range_inverted",
			Message: "close_from must be on or before close_to",
		}
	}
	if f.ValueMin != nil && f.ValueMax != nil && *f.ValueMin > *f.ValueMax {
		return &values.ParseError{
			Field: filterValueMin, Code: "range_inverted",
			Message: "value_min must not be above value_max",
		}
	}
	return nil
}

// refuseMaskedValue refuses a value bound to a role that withholds the amount
// or the currency. The base value carries both, as refuseMaskedSort says.
func refuseMaskedValue(ctx context.Context, f RangeFilters) error {
	parameter := filterValueMin
	if f.ValueMin == nil {
		parameter = filterValueMax
	}
	for _, input := range fieldmask.DealBaseValueInputs() {
		if err := auth.RefuseMaskedFilter(ctx, maskObject, parameter, input); err != nil {
			return err
		}
	}
	return nil
}

// countryFilterClause narrows to deals whose company's address is in the
// country, and only through companies the caller may read.
//
// The country is the company's attribute, so a seat without company.read is
// refused, and the row scope rides inside the EXISTS. Without either, the page
// would tell the caller where a company it cannot open is.
func countryFilterClause(ctx context.Context, country string, arg func(any) int) (string, error) {
	if err := auth.RefuseMaskedFilter(ctx, maskObject, filterCountry, filterCompanyID); err != nil {
		return "", err
	}
	if !auth.ReadGranted(ctx, "company") {
		return "", apperrors.ErrPermissionDenied
	}
	clause := storekit.SQLf(
		"EXISTS (SELECT 1 FROM company country_ref WHERE country_ref.id = deal.company_id AND country_ref.address_country = $%d",
		arg(country))
	scope, err := auth.ScopeClauseFor(ctx, "company", "country_ref", arg)
	if err != nil {
		return "", err
	}
	if scope != "" {
		clause += " AND " + scope
	}
	return clause + ")", nil
}
