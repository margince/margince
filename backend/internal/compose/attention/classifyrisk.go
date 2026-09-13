// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
)

// classifyRisk weighs both value and the time available for deal recovery.
func classifyRisk(item crmcontracts.AttentionItem, asOf time.Time, bar materialBar, money dayMoney) ranked {
	consequence := crmcontracts.WorklistItemConsequence("deal_drifts")
	if item.Kind != nil && *item.Kind == "close_overdue" {
		consequence = "deal_slips_past_close"
	}
	expected, known := expectedRevenue(item, money)
	level := levelAgreed
	material := known && bar.material(expected)
	if material || recoveryDue(item, asOf) {
		level = levelMaterialRisk
	}
	row := base(item, level, "deals_at_risk", consequence)
	priceDealsAtRiskRow(&row, expected, known, money)
	// The reason carries the figure the verdict actually weighed, in the units
	// it was weighed in — the base currency once conversion ran — so a reader
	// comparing it against the summary's threshold compares like with like.
	// The deal's own amount in its own currency still rides on the row's deal
	// facts, so nothing the card states is lost.
	if material {
		row.Because = append(row.Because, reason("material", money.value(expected, item.Deal)))
	} else if known {
		row.Because = append(row.Because, reason("below_material", money.value(expected, item.Deal)))
	}
	// Nobody inside the account is carrying it, which is a different problem
	// from silence and needs a different move: a quiet deal wants a touch, an
	// unchampioned one wants somebody found. Stated whenever the lane knew the
	// answer, at any level — a small deal nobody is arguing for is still a deal
	// nobody is arguing for, and the reason is what the rep acts on.
	if item.Deal != nil && item.Deal.NoChampion != nil && *item.Deal.NoChampion {
		row.Because = append(row.Because, reason("no_champion", nil))
	}
	quiet := quietDaysOf(item)
	if quiet > 0 {
		row.Because = append(row.Because, reason("quiet_days", daysValue(quiet)))
	}
	// A close date calls for recovery or requalification, including when provisional.
	if closingSoon(item, asOf) {
		row.Because = append(row.Because, reason("closing_soon", nil))
	}
	return ranked{
		item: row,
		// The deal's own owner, and it is not known yet: dealfacts fills OwnerId
		// onto the wire AFTER this classification runs. So this lane defers, and
		// the wire step reads the deal the facts pass attached — the same second
		// carrier answersTo has always consulted for these rows.
		ownerRef:         deferredToTheDeal(),
		deadlineAt:       deadlineOf(item.DueAt),
		overdue:          item.Overdue != nil && *item.Overdue,
		expectedBase:     expected,
		hasExpected:      known,
		expectedCurrency: money.base,
		waitingDays:      quiet,
		waitingRank:      orderingAge(quiet),
		occurredAt:       occurredOf(item, asOf),
	}
}

// recoveryDue is a dated recovery decision, independent of portfolio size.
// A provisional close calls for requalification, not a claim that the customer
// committed to that date. The row carries that distinction with its deal facts.
const recoveryHorizonDays = 14

func recoveryDue(item crmcontracts.AttentionItem, asOf time.Time) bool {
	if item.DueAt == nil {
		return false
	}
	horizon := asOf.AddDate(0, 0, recoveryHorizonDays)
	return deadline.Passed(item.DueAt, horizon) || item.DueAt.Equal(horizon)
}

// dealFactsOf carries the deal's own figures onto the queue row. The lane feed
// already resolved them; dropping them here would make the client read a second
// endpoint per row to draw a card this one could have completed.
func dealFactsOf(item crmcontracts.AttentionItem) *crmcontracts.WorklistDealFacts {
	if item.Deal == nil {
		return nil
	}
	facts := &crmcontracts.WorklistDealFacts{
		StageId:              item.Deal.StageId,
		CloseDateProvisional: item.Deal.CloseDateProvisional,
		ForecastCategory:     item.Deal.ForecastCategory,
		OwnerId:              item.Deal.OwnerId,
		AmountMinor:          item.Deal.AmountMinor,
		Currency:             item.Deal.Currency,
		// A finding or nothing. `false` is never sent, so a covered committee
		// reaches the wire absent alongside the unreadable and the seatless one
		// — a reader who cannot see the seats must not be able to tell those
		// apart, because telling them apart is the disclosure.
		NoChampion: aFindingOnly(item.Deal.NoChampion),
	}
	// The close date rides on the lane item's own due moment, and the idle
	// count on its own typed field. Both were already resolved; only this projection
	// dropped them, so the card could state money and never say when the deal
	// was meant to land.
	if item.DueAt != nil {
		closes := openapi_types.Date{Time: *item.DueAt}
		facts.ExpectedCloseDate = &closes
	}
	if quiet := quietDaysOf(item); quiet > 0 {
		facts.QuietDays = &quiet
	}
	return facts
}

// aFindingOnly drops a stated false, so absence is this field's only negative.
//
// Both producers of the champion answer call it, because one rule spelled twice
// drifts: a covered committee, an unreadable one and a seatless deal must reach
// the wire alike, and a reader able to tell them apart is the disclosure the
// rule exists to refuse.
func aFindingOnly(answer *bool) *bool {
	if answer == nil || !*answer {
		return nil
	}
	return answer
}

// expectedRevenue is what the deal is worth times how likely it is to land.
//
// The win probability lives on the stage rather than the deal, and this feed
// does not read stages — so until that read exists the amount stands in for the
// expectation, and it will get more accurate rather than change meaning.
//
// Once conversion ran, the answer is the deal's amount in the installation's
// base currency — the only figure by which two deals may be compared — and a
// deal the estate could not price answers unknown rather than a raw number in
// the wrong units. Before conversion (an assembly without the FX seam) the raw
// amount stands, comparable only while every deal shares one currency.
func expectedRevenue(item crmcontracts.AttentionItem, money dayMoney) (int64, bool) {
	if item.Deal == nil || item.Deal.AmountMinor == nil {
		return 0, false
	}
	if !money.converted() {
		return *item.Deal.AmountMinor, true
	}
	// The same guard dayMoney.value already holds for the reason field: a
	// converted figure with no base currency named is not a smaller answer,
	// it is no answer, and must not drive a material verdict either.
	if money.base == "" {
		return 0, false
	}
	converted, priced := money.byItem[item.Id]
	return converted, priced
}

// priceDealsAtRiskRow writes the contract's per-row ExpectedMinorBase, the
// one place this guard is spelled: classifyRisk and classifyBriefItem both
// produce a "deals_at_risk" row for a deal, and a second copy of the guard is
// a second answer to "is this row priced" that the two could drift apart on.
//
// Set only once conversion actually ran (money.converted() is the raw-amount
// guard) and the deal WAS priced — expected/known already answer false for a
// deal with no amount, an unpriced converted item, or a converted figure with
// no base currency named, so this is the one place that answer is spent
// rather than a second copy of any of its three reasons.
func priceDealsAtRiskRow(row *crmcontracts.WorklistItem, expected int64, known bool, money dayMoney) {
	if row.Deal != nil && money.converted() && known {
		row.Deal.ExpectedMinorBase = &expected
	}
}

// moneyOf carries the deal's own currency beside the amount. An amount without
// one is not a smaller answer, it is no answer: the client refuses to format a
// figure it cannot name the units of, so a reason that omits the currency
// reaches the reader as a bare "material" with the number silently dropped.
func moneyOf(minor int64, deal *crmcontracts.AttentionDealFacts) *crmcontracts.WorklistValue {
	value := minor
	money := &crmcontracts.WorklistValue{Kind: valueMoney, Minor: &value}
	if deal != nil {
		money.Currency = deal.Currency
	}
	return money
}

// The overdue flag is the deal engine's calendar-aware verdict. A past close
// date must not be described as an expected close in the coming fortnight.
func closingSoon(item crmcontracts.AttentionItem, asOf time.Time) bool {
	return recoveryDue(item, asOf) && (item.Overdue == nil || !*item.Overdue)
}
