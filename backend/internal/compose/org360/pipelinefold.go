// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

// What the account's open deals add up to, and what the page may honestly say
// about that figure (plan §4.2). Separate from the query that scans them so the
// money rules can be proven without a database.

import (
	"math"
	"slices"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// openRow is one open deal as the pipeline read scans it.
// openDeal is one open deal as the advice cites it: what it is called, and the
// record a reader opens to check.
type openDeal struct {
	ID   ids.UUID
	Name string
}

type openRow struct {
	id         ids.UUID
	name       string
	stalled    bool
	idleSince  time.Time
	stageMoves int
	// amountMinor is the deal's own figure in its own currency; nil when the
	// deal names no amount at all.
	amountMinor *int64
	// valueBase is the deal's amount converted to the base currency, at the
	// latest rate on or before the read's as-of day. Every row here is OPEN, so
	// there is no frozen rate to prefer: the rate freezes on close
	// (deals.deal_advance). It is null when the installation holds no usable
	// rate for the pair, which is the case this fold refuses.
	valueBase *int64
	closeOn   *time.Time
	currency  *string
	rateDate  *time.Time
	baseCcy   string
}

// baseValueOf answers what one open deal contributes to the base-currency
// total, and whether a conversion stood behind it.
//
// A deal ALREADY in the base currency needs no rate, and contributes its own
// figure. This is the ordinary case and the reason the fold cannot read
// amount_minor_base: that column is null on every open deal, because the rate
// freezes on CLOSE (deals.deal_advance).
//
// A deal in another currency contributes only when the converted figure AND the
// date of the rate that produced it are both present. Refusing it otherwise is
// what keeps §4.2's rule true: a converted figure always has a conversion
// behind it and a date to name. What is refused is the honestly unpriceable
// deal — one whose currency the installation holds no rate for on or before the
// as-of day, whether because none was ever loaded or because every rate it has
// is dated later. Such a deal still counts toward open_count, so the page
// reports a total covering part of the pipeline rather than a silently short
// one.
func baseValueOf(deal openRow) (value int64, converted bool, ok bool) {
	if deal.currency != nil && *deal.currency == deal.baseCcy {
		if deal.amountMinor == nil {
			return 0, false, false
		}
		return *deal.amountMinor, false, true
	}
	if deal.valueBase == nil || deal.rateDate == nil {
		return 0, false, false
	}
	return *deal.valueBase, true, true
}

// wouldWrap reports whether adding value to running would carry the sum past
// what an int64 can hold.
//
// Each conversion is bounded on its own — Postgres refuses a round() that does
// not fit a bigint — but a sum of valid amounts is not. Checked before the add
// rather than after: once it has wrapped, the result is a number in range and
// there is nothing left to notice.
func wouldWrap(running, value int64) bool {
	if value > 0 {
		return running > math.MaxInt64-value
	}
	return running < math.MinInt64-value
}

// foldPipeline turns the scanned rows into the figures the page reports.
//
// Separate from the query so the MONEY rules can be proven without a database:
// which deals enter the total, what the total means when some cannot, and what
// provenance a converted figure has to carry (plan §4.2).
func foldPipeline(open []openRow) pipeline {
	out := pipeline{
		OpenCount: len(open),
		Stalled:   make([]stalledDeal, 0, len(open)),
		Open:      make([]openDeal, 0, len(open)),
	}
	sorted := make([]string, 0, len(open))
	for _, deal := range open {
		sorted = append(sorted, deal.id.String())
		out.Open = append(out.Open, openDeal{ID: deal.id, Name: deal.name})
		// A total that wraps past int64 is a plausible-looking wrong number,
		// which is worse than no number. Such a deal stays counted in
		// open_count and out of the sum, exactly as an unconvertible one does,
		// so priced_count reports that the figure covers part of the pipeline
		// rather than all of it.
		if value, converted, ok := baseValueOf(deal); ok && !wouldWrap(out.ValueMinorBase, value) {
			out.ValueMinorBase += value
			out.Priced++
			out.BaseCurrency = deal.baseCcy
			if converted {
				out.Converted++
				// Only reached when a rate date exists: baseValueOf refuses a
				// converted figure without one, so a converted total always has
				// an as-of date behind it (plan §4.2).
				if out.FXAsOf == nil || deal.rateDate.Before(*out.FXAsOf) {
					out.FXAsOf = deal.rateDate
				}
			}
		}
		if deal.closeOn != nil && (out.NextCloseOn == nil || deal.closeOn.Before(*out.NextCloseOn)) {
			out.NextCloseOn = deal.closeOn
		}
		if deal.stalled {
			out.Stalled = append(out.Stalled, stalledDeal{
				ID: deal.id, Name: deal.name,
				IdleSince: deal.idleSince, StageMoves: deal.stageMoves,
			})
		}
	}
	// Sorted by id rather than by the read's order, so the digest depends on
	// WHICH deals are open and on nothing else — a deal whose last activity moves
	// must not read as a changed pipeline.
	slices.Sort(sorted)
	out.OpenDigest = strings.Join(sorted, ",")
	return out
}
