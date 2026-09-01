// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Putting the day's money into ONE currency before the ordering reads it.
//
// The expected-revenue tie-break and the material bar both compare amounts
// between deals, and raw minor units compare a yen to a euro and get it wrong:
// ¥1,000,000 outranks €50,000 as an integer while being worth a twentieth of
// it. So the amounts the ordering weighs are converted to the installation's
// base currency first, through the seam below — the same engine the company
// page and the hierarchy rollup already price with, so the queue cannot learn
// a second answer to "what is this deal worth here".
//
// A deal the estate cannot price — no stored rate for its currency, or an
// amount with no currency at all — ranks as UNPRICED, exactly as a deal whose
// value nobody recorded: absence of a comparable figure is not a large figure,
// and a raw number in the wrong units is not a smaller error than none.

import (
	"context"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// CurrencyAmount is one figure in the currency its own record carries — the
// input side of the conversion, before it is comparable to anything.
type CurrencyAmount struct {
	Minor    int64
	Currency string
}

// BaseMoney answers what amounts are worth in the installation's base currency.
//
// One batched call per read, like DealFacts: the day names its amounts once and
// the seam prices them together, rather than a rate lookup per row.
type BaseMoney interface {
	// ToBase converts each amount, answering a slice parallel to the input:
	// nil where the estate holds no rate for that currency, or where the
	// product would not fit money's range — the caller ranks such a deal as
	// unpriced rather than by a number in the wrong units. The second result
	// names the base currency every non-nil figure is stated in.
	ToBase(ctx context.Context, asOf time.Time, amounts []CurrencyAmount) ([]*int64, string, error)
}

// dayMoney is one read's conversion answer, threaded into the classifiers.
//
// The zero value means the FX seam is unbound: figures stay in raw minor units
// and nothing claims a currency for them — the shape this projection had before
// the seam existed, kept so an assembly without the binding degrades to a
// smaller promise rather than a wrong one.
type dayMoney struct {
	// base is the currency every priced figure is stated in; empty when no
	// conversion ran.
	base string
	// byItem is each priced lane item's expected revenue in the base currency,
	// keyed by the item's id. An at-risk item absent here, under a non-empty
	// base, is one the estate could not price.
	byItem map[string]int64
}

// converted reports whether figures went through the base currency at all —
// the difference between "these numbers share units" and "these numbers are
// whatever each deal happened to say".
func (m dayMoney) converted() bool { return m.base != "" }

// value states an expected-revenue figure in the units it is genuinely in:
// the base currency once conversion ran, the deal's own before that. Either
// way the number and its units travel together — a figure without units
// reaches the reader as a bare verdict with the amount silently dropped.
func (m dayMoney) value(minor int64, deal *crmcontracts.AttentionDealFacts) *crmcontracts.WorklistValue {
	if !m.converted() {
		return moneyOf(minor, deal)
	}
	value := minor
	currency := m.base
	return &crmcontracts.WorklistValue{Kind: valueMoney, Minor: &value, Currency: &currency}
}

// priceDayOnto prices the day and keeps the answer on THIS service value.
//
// The caller is always a per-request copy (forReader, forOwner and their
// siblings in feed.go), never the shared Service: one Service serves every
// request, and a day's rates written to it would price another reader's page
// at another moment's reading. Writing the field here rather than at the call
// site is what keeps that pairing in one place.
//
// A failed conversion fails the READ. A page ranked on raw cross-currency
// numbers is the defect this seam exists to end, not a degraded mode to fall
// back into — the unconverted path below is for an assembly with no seam
// bound at all, which production never is.
func (s *Service) priceDayOnto(ctx context.Context, day crmcontracts.Attention) error {
	money, err := s.priceTheDay(ctx, day)
	if err != nil {
		return err
	}
	s.money = money
	return nil
}

// priceTheDay converts the amounts the ordering is about to compare.
//
// Only the at-risk lane feeds the expected-revenue tie-break and the material
// bar, so only its amounts are priced. An unbound seam, or a day with nothing
// to price, answers the zero dayMoney and costs no read.
func (s *Service) priceTheDay(ctx context.Context, day crmcontracts.Attention) (dayMoney, error) {
	if s.fx == nil || day.AtRisk == nil {
		return dayMoney{}, nil
	}
	items := make([]string, 0, len(*day.AtRisk))
	amounts := make([]CurrencyAmount, 0, len(*day.AtRisk))
	for _, item := range *day.AtRisk {
		// An amount with no currency cannot be converted, and under a bound
		// seam it cannot be compared either: it is left out here, so the item
		// ranks as unpriced rather than as a number in unknowable units.
		if item.Deal == nil || item.Deal.AmountMinor == nil || item.Deal.Currency == nil {
			continue
		}
		items = append(items, item.Id)
		amounts = append(amounts, CurrencyAmount{Minor: *item.Deal.AmountMinor, Currency: *item.Deal.Currency})
	}
	if len(amounts) == 0 {
		return dayMoney{}, nil
	}
	converted, base, err := s.fx.ToBase(ctx, day.AsOf, amounts)
	if err != nil {
		return dayMoney{}, err
	}
	money := dayMoney{base: base, byItem: make(map[string]int64, len(converted))}
	for i, figure := range converted {
		if figure != nil {
			money.byItem[items[i]] = *figure
		}
	}
	return money, nil
}

// WithBaseMoney binds the conversion that puts the day's amounts into the
// installation's base currency before the ordering compares them.
//
// An option for the reason WithWaiting is one, and it lives HERE rather than
// beside the other options because everything it binds is in this file: the
// seam, the per-read answer, and the pass that fills it. A reader asking how
// the queue gets its money finds one file rather than two.
//
// Unbound, amounts stay in raw minor units — comparable only while every deal
// shares one currency, which is why production wiring always binds this.
func (s *Service) WithBaseMoney(fx BaseMoney) *Service {
	s.fx = fx
	return s
}
