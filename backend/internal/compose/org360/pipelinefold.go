// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

// What the account's open deals add up to, and what the page may honestly say
// about that figure (plan §4.2). Separate from the query that scans them so the
// money rules can be proven without a database.

import (
	"slices"
	"strings"
	"time"

	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// openRow is one open deal as the pipeline read scans it.
type openRow struct {
	id         ids.UUID
	name       string
	stalled    bool
	idleSince  time.Time
	stageMoves int
	// amountMinor is the deal's own figure in its own currency; nil when the
	// deal names no amount at all.
	amountMinor *int64
	// valueBase is the CONVERTED figure. On a closed deal it is the frozen
	// column; on an open one the read converts at the latest rate on or before
	// its as-of day, because the rate freezes only on close
	// (deals.deal_advance) and the column is null until then. It stays null
	// when no rate exists for the pair, which is the case this fold refuses.
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
// figure. This is the ordinary case and the reason the fold cannot simply read
// amount_minor_base: that generated column is null on every open deal, because
// the rate freezes on CLOSE (deals.deal_advance).
//
// A deal in another currency contributes only when a converted figure AND its
// rate date are both present — a frozen pair on a closed deal, or the latest
// stored rate the read applied to an open one. Refusing it otherwise is what
// keeps §4.2's rule true: a converted figure always has a conversion behind it
// and a date to name. What is refused now is only the honestly unpriceable
// case, a currency the installation holds no rate for at all, and such a deal
// still counts toward open_count — so the page reports a total covering part of
// the pipeline rather than a silently short one.
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

// foldPipeline turns the scanned rows into the figures the page reports.
//
// Separate from the query so the MONEY rules can be proven without a database:
// which deals enter the total, what the total means when some cannot, and what
// provenance a converted figure has to carry (plan §4.2).
func foldPipeline(open []openRow) pipeline {
	out := pipeline{OpenCount: len(open), Stalled: make([]stalledDeal, 0, len(open))}
	sorted := make([]string, 0, len(open))
	for _, deal := range open {
		sorted = append(sorted, deal.id.String())
		if value, converted, ok := baseValueOf(deal); ok {
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
