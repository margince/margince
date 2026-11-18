// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"cmp"
	"encoding/json"
	"slices"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/closerisk"
	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
	"github.com/margince/margince/backend/internal/shared/kernel/goalgap"
)

// FocusCap is the most deals one focus list names. A manager works a short
// list; a longer one is the pipeline again.
const FocusCap = 20

// FocusReason is why a deal is on a focus list, in facts the snapshot stores
// beside it. It never carries the score, so a reader can check every part.
type FocusReason struct {
	// ClosesInDays is how many days after the snapshot's day the deal is
	// expected to close, negative once that date has passed.
	ClosesInDays     *int     `json:"closes_in_days,omitempty"`
	CloseProvisional bool     `json:"close_provisional,omitempty"`
	Signals          []Signal `json:"signals,omitempty"`
}

// Focus is one deal's place on a focus list, from 1.
type Focus struct {
	Rank   int
	Reason FocusReason
}

// FocusList is the deals a team works to meet its goal, by deal id. OnTarget
// says the expected landing already met the goal, so the list ranks the deals
// whose close dates put that at risk.
type FocusList struct {
	OnTarget bool
	Deals    map[string]Focus
}

// RankFocus ranks the open priced deals of readings as of asOf, counting days
// in asOf's zone. gap is the goal less the expected landing. Behind the goal it
// ranks by weighted amount, raised for each close-date risk. On target it
// lists only deals with a close-date risk, most risks first. Ties go by deal
// id, so one set of readings always gives one list.
func RankFocus(readings Readings, gap int64, asOf time.Time) FocusList {
	type candidate struct {
		id     string
		score  int64
		risks  int64
		reason FocusReason
	}
	onTarget := gap <= 0
	var ranked []candidate
	for _, c := range readings.Contributions {
		if !c.InOpen || c.InWon || c.BaseMinor == nil || c.WeightedMinor <= 0 {
			continue
		}
		reason := FocusReason{CloseProvisional: c.CloseProvisional, Signals: c.Signals, ClosesInDays: closesInDays(asOf, c.EffectiveClose)}
		risks := closeRisks(reason)
		if onTarget && risks == 0 {
			continue
		}
		ranked = append(ranked, candidate{id: c.DealID, score: c.WeightedMinor * (1 + risks), risks: risks, reason: reason})
	}
	slices.SortFunc(ranked, func(a, b candidate) int {
		if onTarget {
			if byRisk := cmp.Compare(b.risks, a.risks); byRisk != 0 {
				return byRisk
			}
		}
		return cmp.Or(cmp.Compare(b.score, a.score), cmp.Compare(a.id, b.id))
	})
	out := FocusList{OnTarget: onTarget, Deals: map[string]Focus{}}
	for i, entry := range ranked[:min(len(ranked), FocusCap)] {
		out.Deals[entry.id] = Focus{Rank: i + 1, Reason: entry.reason}
	}
	return out
}

// closesInDays is how many days after asOf's local day closes falls, counted
// in asOf's zone; nil without a close date. The freeze and every later read
// count it the same way.
func closesInDays(asOf time.Time, closes *time.Time) *int {
	if closes == nil {
		return nil
	}
	day := time.Date(closes.Year(), closes.Month(), closes.Day(), 0, 0, 0, 0, asOf.Location())
	return new(elapsed.Days(asOf, day, asOf.Location()))
}

// closeRisks counts the facts that say a deal may not close on its date, by
// the Worklist's own rule in closerisk. The date passed or is a guess. The
// date kept moving, or is near with no next step booked.
func closeRisks(reason FocusReason) int64 {
	var risks int64
	if reason.ClosesInDays != nil && closerisk.Passed(*reason.ClosesInDays) {
		risks++
	}
	if reason.CloseProvisional {
		risks++
	}
	near := reason.ClosesInDays != nil && closerisk.Near(*reason.ClosesInDays)
	for _, signal := range reason.Signals {
		code := crmcontracts.ForecastRiskSignalCode(signal.Code)
		if code == crmcontracts.ForecastRiskSignalCodeClosePushed ||
			(code == crmcontracts.ForecastRiskSignalCodeNoNextStep && near) {
			risks++
		}
	}
	return risks
}

// focusOf ranks a team snapshot's deals against the landing it freezes, so
// the gap and the landing on the snapshot are one figure. A snapshot with no
// goal, or no landing, ranks none.
func focusOf(in NewSnapshot, landing Landing) *FocusList {
	if in.FocusGoal == nil || landing.Measure == "" {
		return nil
	}
	list := RankFocus(in.Readings, goalgap.Of(*in.FocusGoal, landing.AmountMinor), in.TakenAt.In(in.Period.Zone))
	return &list
}

// focusColumns is a deal's focus_rank and focus_reason, both NULL off the list.
func focusColumns(focus *FocusList, dealID string) (*int, []byte, error) {
	if focus == nil {
		return nil, nil, nil
	}
	entry, listed := focus.Deals[dealID]
	if !listed {
		return nil, nil, nil
	}
	reason, err := json.Marshal(entry.Reason)
	return &entry.Rank, reason, err
}
