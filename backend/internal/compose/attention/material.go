// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What makes a drifting deal worth interrupting the day for.
//
// "Material" cannot be a constant. A workspace whose deals are five figures and
// one whose deals are seven would need different numbers, and a number typed
// into a settings screen is one nobody revisits as the pipeline grows. So the
// bar is the pipeline's OWN median: half the deals at risk clear it, and it
// moves when the business does.
//
// The alternative considered and rejected: treat every priced deal as material.
// That is what the first cut did, and it put a one-euro deal above an overdue
// customer task — the ordering a reader would call obviously wrong.

import (
	"sort"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// materialBar is the threshold this read judges against, and whether there was
// enough of a pipeline to take one from.
type materialBar struct {
	minor int64
	known bool
	// currency names the units the bar is stated in — the base currency when
	// the amounts it was taken from were converted, empty when they were raw
	// minor units and the bar is not genuinely in any one currency.
	currency string
}

// material reports whether an expected figure clears the bar.
//
// Strictly above the median, not equal to it. With an odd count the median IS
// one of the deals, and admitting it would put a typical deal in the band
// reserved for the ones worth interrupting a day — three deals of 1, 2 and 160
// would make the 2 material. "Material" has to mean better than typical.
//
// A workspace with no priced deals at risk has no median, and then nothing is
// material — the honest answer rather than promoting everything.
func (b materialBar) material(expected int64) bool {
	return b.known && expected > b.minor
}

// materialBarOf takes the median of the priced deals this reader can see.
//
// The median rather than the mean: one very large deal must not lift the bar
// above every other deal in the pipeline, which is exactly what an average does
// and exactly the shape a sales pipeline has.
func materialBarOf(day crmcontracts.Attention, money dayMoney) materialBar {
	if day.AtRisk == nil {
		return materialBar{}
	}
	amounts := make([]int64, 0, len(*day.AtRisk))
	for _, item := range *day.AtRisk {
		if expected, known := expectedRevenue(item, money); known {
			amounts = append(amounts, expected)
		}
	}
	if len(amounts) == 0 {
		return materialBar{}
	}
	sort.Slice(amounts, func(i, j int) bool { return amounts[i] < amounts[j] })
	// The LOWER median, so that "above the median" admits the upper half rather
	// than excluding it. With an even count the upper-middle value is itself a
	// deal, and taking it would leave the largest deal in a two-deal pipeline
	// below the bar — which is how a €160k deal came back "below material" on a
	// day it was the biggest thing the rep had.
	return materialBar{minor: amounts[(len(amounts)-1)/2], known: true, currency: money.base}
}

// stateOn writes the bar onto the summary the header draws.
//
// Why a deal ranked where it did, in the figure the ranking actually used. The
// contract has promised the threshold since the queue shipped and the
// projection never sent it, so every "material" and "below material" reason on
// the page was a verdict with its threshold withheld: a reader could see that a
// deal had been called big, and had no way to ask compared to what.
//
// base_currency travels only when the amounts genuinely went through one
// currency. Without the FX seam the bar is a median of raw amount_minor values
// in no one currency, and naming one would assert a conversion that did not
// happen — worse than a number the client formats cautiously.
func (b materialBar) stateOn(summary *crmcontracts.WorklistSummary) {
	if !b.known {
		return
	}
	minor := b.minor
	summary.MaterialThresholdMinor = &minor
	if b.currency != "" {
		currency := b.currency
		summary.BaseCurrency = &currency
	}
}
