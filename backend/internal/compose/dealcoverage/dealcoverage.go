// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dealcoverage says why a deal's money is missing from a converted
// total: no amount was entered, or the amount's currency has no rate. It
// renders the counts in the same statement as the sums they qualify.
package dealcoverage

import (
	"slices"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/moneytotal"
)

// Key is the row key a report puts its coverage under.
const Key = "coverage"

const (
	colNoValue    = "coverage_no_value"
	colNoRate     = "coverage_no_rate"
	colCurrencies = "coverage_currencies"
)

// NoValueSQL holds for a deal, under the table alias given, with no amount.
func NoValueSQL(alias string) string { return moneytotal.NoValueSQL(alias + ".amount_minor") }

// NoRateSQL holds for a deal with an amount that baseValueSQL cannot convert.
func NoRateSQL(alias, baseValueSQL string) string {
	return moneytotal.NoRateSQL(alias+".amount_minor", baseValueSQL)
}

// Counts is what moneytotal.CoverageSQL reads back.
type Counts struct {
	NoValue, NoRate int64
	Currencies      *string
}

// Dest lists the scan targets for the coverage terms, in order.
func (c *Counts) Dest() []any { return []any{&c.NoValue, &c.NoRate, &c.Currencies} }

// Wire states the counts as the contract's coverage, naming the figures they
// qualify.
func (c Counts) Wire(figures ...string) crmcontracts.ReportRowCoverage {
	currencies := []string{}
	if c.Currencies != nil && *c.Currencies != "" {
		currencies = strings.Split(*c.Currencies, ",")
	}
	return crmcontracts.ReportRowCoverage{
		Aggregates: figures, Partial: c.NoValue+c.NoRate > 0,
		NoValueDeals: int(c.NoValue), NoRateDeals: int(c.NoRate), MissingCurrencies: currencies,
	}
}

// ScopedTerms is the coverage terms for a total over more deals than the reader
// may see. The counts and the currencies cover only the deals visibleSQL admits,
// and an empty visibleSQL admits every deal. A last term counts the deals left
// out anywhere. The total can then say it is partial without saying how many
// hidden deals are behind it.
func ScopedTerms(valueSQL, amountSQL, currencySQL, visibleSQL string) string {
	if visibleSQL == "" {
		visibleSQL = "TRUE"
	}
	anyGap := "count(*) FILTER (WHERE " + moneytotal.NoValueSQL(amountSQL) + " OR " + moneytotal.NoRateSQL(amountSQL, valueSQL) + ")"
	return strings.Join(append(moneytotal.CoverageTerms(valueSQL, amountSQL, currencySQL, visibleSQL), anyGap), ", ")
}

// ScopedCounts is what ScopedTerms reads back.
type ScopedCounts struct {
	Counts
	AnyGap int64
}

// Dest lists the scan targets for ScopedTerms, in order.
func (c *ScopedCounts) Dest() []any { return append(c.Counts.Dest(), &c.AnyGap) }

// Wire states the visible counts, and partial when any deal is left out.
func (c ScopedCounts) Wire(figures ...string) crmcontracts.ReportRowCoverage {
	wire := c.Counts.Wire(figures...)
	wire.Partial = c.AnyGap > 0
	return wire
}

// Attach adds, after the plan's own terms and columns, the terms that count a
// group's deals left out of the partial sums named in covered. With none named
// the plan is returned as it came.
func Attach(covered []string, baseValueSQL, currencySQL string, selects, columns []string) (outSelects, outColumns []string) {
	if len(covered) == 0 {
		return selects, columns
	}
	terms := moneytotal.CoverageTerms(baseValueSQL, "t.amount_minor", currencySQL, "TRUE")
	return append(slices.Clip(selects), terms...),
		append(slices.Clip(columns), colNoValue, colNoRate, colCurrencies)
}

// Fold moves each row's coverage columns into one `coverage` object naming the
// aggregates it qualifies.
func Fold(rows []map[string]any, aggregates []string) {
	if len(aggregates) == 0 {
		return
	}
	for _, row := range rows {
		noValue, noRate := count(row[colNoValue]), count(row[colNoRate])
		currencies := []string{}
		if listed, ok := row[colCurrencies].(string); ok && listed != "" {
			currencies = strings.Split(listed, ",")
		}
		delete(row, colNoValue)
		delete(row, colNoRate)
		delete(row, colCurrencies)
		row[Key] = map[string]any{
			"aggregates":         aggregates,
			"partial":            noValue+noRate > 0,
			"no_value_deals":     noValue,
			"no_rate_deals":      noRate,
			"missing_currencies": currencies,
		}
	}
}

//craft:ignore naked-any a driver row value of unknown integer width
func count(value any) int64 {
	switch n := value.(type) {
	case int64:
		return n
	case int32:
		return int64(n)
	case int:
		return int64(n)
	}
	return 0
}
