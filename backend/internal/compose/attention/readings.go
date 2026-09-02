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
func readingsOf(
	considered []ranked, bounds map[crmcontracts.WorklistItemSource]bool,
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
			priced = true
			// Every priced row on one read shares one answer, because one
			// conversion priced them all. Taking it from the row rather than
			// from the service keeps the currency travelling with the figure it
			// names, which is the pairing the money values already hold.
			currency = row.expectedCurrency
		}
	}
	if priced {
		out.RevenueAtRiskMinor = &revenue
		if currency != "" {
			out.RevenueCurrency = &currency
		}
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
