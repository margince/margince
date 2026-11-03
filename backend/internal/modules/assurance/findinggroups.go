// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"cmp"
	"slices"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// FindingGroup is the findings of one type in a list.
type FindingGroup struct {
	Type          string
	Findings      int
	Deals         int
	OtherFindings int
	// Exposure is one amount per currency, in currency order.
	Exposure []Exposure
	// Answerable says whether the caller may answer at least one finding.
	Answerable bool
}

// Exposure is the money a group puts in question in one currency.
type Exposure struct {
	Currency    string
	AmountMinor int64
}

// FindingTotals counts a whole list, each deal once.
type FindingTotals struct {
	Findings      int
	Deals         int
	OtherFindings int
}

// GroupFindings groups a list by type, in the order each type first appears.
// So the first group is the one the list's ranking puts first. A deal with two
// findings of one type is one deal. Its money counts once, at the larger
// amount, because both findings name the same deal's money.
func GroupFindings(found []Exception, answerable map[ids.UUID]bool) ([]FindingGroup, FindingTotals) {
	var order []string
	groups := map[string]*groupTally{}
	allDeals := map[ids.UUID]struct{}{}
	totals := FindingTotals{Findings: len(found)}
	for _, e := range found {
		tally, seen := groups[e.Type]
		if !seen {
			tally = &groupTally{dealMoney: map[ids.UUID]Exposure{}}
			groups[e.Type] = tally
			order = append(order, e.Type)
		}
		tally.findings++
		tally.answerable = tally.answerable || answerable[e.ID]
		if e.SubjectKind != subjectKindDeal {
			tally.other++
			totals.OtherFindings++
			continue
		}
		allDeals[e.SubjectID] = struct{}{}
		tally.addDeal(e)
	}
	totals.Deals = len(allDeals)
	out := make([]FindingGroup, 0, len(order))
	for _, kind := range order {
		out = append(out, groups[kind].group(kind))
	}
	return out, totals
}

type groupTally struct {
	findings, other int
	answerable      bool
	// dealMoney holds each deal's largest amount; a deal with no amount is
	// still a key, so it is counted as a deal.
	dealMoney map[ids.UUID]Exposure
}

func (g *groupTally) addDeal(e Exception) {
	held, seen := g.dealMoney[e.SubjectID]
	if e.AffectedMinor == nil || e.Currency == "" {
		if !seen {
			g.dealMoney[e.SubjectID] = Exposure{}
		}
		return
	}
	if !seen || held.Currency == "" || *e.AffectedMinor > held.AmountMinor {
		g.dealMoney[e.SubjectID] = Exposure{Currency: e.Currency, AmountMinor: *e.AffectedMinor}
	}
}

func (g *groupTally) group(kind string) FindingGroup {
	byCurrency := map[string]int64{}
	for _, money := range g.dealMoney {
		if money.Currency != "" {
			byCurrency[money.Currency] += money.AmountMinor
		}
	}
	exposure := make([]Exposure, 0, len(byCurrency))
	for currency, amount := range byCurrency {
		exposure = append(exposure, Exposure{Currency: currency, AmountMinor: amount})
	}
	slices.SortFunc(exposure, func(a, b Exposure) int { return cmp.Compare(a.Currency, b.Currency) })
	return FindingGroup{
		Type: kind, Findings: g.findings, Deals: len(g.dealMoney), OtherFindings: g.other,
		Exposure: exposure, Answerable: g.answerable,
	}
}
