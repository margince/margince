// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The four figures the strip above the queue states.
//
// They answer a different question from the filter pills, which is why both are
// on the page: `counts` says how many items of a kind the queue holds, and these
// say what those items mean for the day. That is why one of the four is a sum of
// money rather than a tally — "eleven deals at risk" and "€380k drifting" are not
// the same news, and the second is the one that decides whether a rep opens the
// pill.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// readingsOf states the day's four outcome figures over the set `considered`
// holds.
//
// That set — after the scope narrowing, before the category filter, the fold and
// the page cut — is the only one for which these numbers are stable. Two
// alternatives were tried against the shape of the screen and both lie:
//
//   - over the PAGE, the strip shrinks as a reader walks the queue, so work reads
//     as disappearing while they do it;
//   - after the FILTER, opening one pill empties the other three, so the strip
//     answers a question nobody asked.
//
// `counts` already settled this for the same reason, and takes the same snapshot.
//
// The money is summed from what the ranking already priced rather than recomputed
// here. `expectedBase` is the deal's amount in the base currency, decided once by
// the conversion seam, and a second sum over raw amounts would be a second answer
// to what a deal is worth — in the units where a yen outranks a euro.
//
// `unread` is the day's own list of lanes that never answered — refused to this
// reader, or failed. It is SEPARATE from `bounds` because the two are different
// facts and only one of them is in that map: `bounds` records lanes that ran, so
// a lane which never ran is simply absent from it. Reading the floor flag from
// `bounds` alone therefore states exact figures over work nobody looked at, which
// is the one direction these numbers must never fail in.
func readingsOf(
	considered []ranked,
	bounds map[crmcontracts.WorklistItemSource]bool,
	unread []crmcontracts.WorklistSourceUnavailable,
) crmcontracts.WorklistReadings {
	out := crmcontracts.WorklistReadings{}
	// A priced deal is one the estate could state a comparable figure for. An
	// unpriced one is LEFT OUT rather than added as zero: a deal nobody recorded
	// an amount for is not a deal worth nothing, and counting it as nothing
	// reports a safer pipeline than the one that exists.
	var revenue int64
	var priced bool
	var currency string
	for _, row := range considered {
		switch row.item.Category {
		case crmcontracts.WorklistItemCategoryCustomerWaiting:
			out.BuyerReplies++
		case categoryLeads:
			out.Prospecting++
		case categoryDecisions:
			// Counted from `considered`, which is held before the fold, so a
			// hundred alike approvals read as a hundred here even where the queue
			// draws them as one row. The strip says how much work there is; the
			// queue says how much reading it costs.
			out.Review++
		case crmcontracts.WorklistItemCategoryDealsAtRisk:
			if !row.hasExpected {
				continue
			}
			revenue += row.expectedBase
			// Taking the units from the ROW keeps the currency travelling with
			// the figure it names, which is the pairing the money values already
			// hold. One conversion prices a whole read, so today every priced row
			// carries the same answer — but that is a fact about the current
			// classifier rather than a guarantee, and a later one setting the
			// field from somewhere else would silently label the sum with
			// whichever row happened to come last. Disagreement therefore refuses
			// to name a currency at all, and the client draws no money figure it
			// cannot state the units of.
			switch {
			case !priced:
				currency = row.expectedCurrency
			case currency != row.expectedCurrency:
				currency = ""
			}
			priced = true
		}
	}
	if priced {
		out.RevenueAtRiskMinor = &revenue
		if currency != "" {
			out.RevenueCurrency = &currency
		}
	}
	// A lane that never answered makes every figure a floor just as surely as one
	// that answered to its cap — more so, since nobody knows what it held. This
	// arm is FIRST because it is the one a reader loses silently: a refused lane
	// leaves no rows to notice missing, so the strip reads as a clear day rather
	// than as a day nobody could see.
	if len(unread) > 0 {
		out.MoreAvailable = true
		return out
	}
	// One flag for the row rather than one per figure. The four are read across
	// as a single statement about the day, so a reader who cannot trust one of
	// them cannot trust the row — and a strip with three exact numbers and one
	// floor invites exactly the reading where the floor is the one overlooked.
	for _, bounded := range bounds {
		if bounded {
			out.MoreAvailable = true
			break
		}
	}
	return out
}
