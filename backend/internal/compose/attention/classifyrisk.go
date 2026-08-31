// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What a drifting deal is WORTH, and whether that is worth interrupting a day
// for.
//
// Apart from the other classifiers because it is the only one that answers a
// question about money rather than about time: everything else on the queue
// ranks on a clock, and this one ranks on an amount weighed against what the
// pipeline itself normally carries.

import (
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// classifyRisk: a deal drifting. Whether it is worth interrupting the day for
// is decided against the pipeline's own median rather than a number somebody
// typed once, so "material" tracks the business as it changes.
func classifyRisk(item crmcontracts.AttentionItem, asOf time.Time, bar materialBar) ranked {
	consequence := crmcontracts.WorklistItemConsequence("deal_drifts")
	if item.Kind != nil && *item.Kind == "close_overdue" {
		consequence = "deal_slips_past_close"
	}
	expected, known := expectedRevenue(item)
	// Material revenue interrupts the day; a smaller deal drifting is agreed
	// work like any other. The bar is the pipeline's own median rather than a
	// number somebody typed once, so "material" tracks the business as it
	// moves — and a deal whose value nobody recorded is not assumed large.
	level := levelAgreed
	if known && bar.material(expected) {
		level = levelMaterialRisk
	}
	row := base(item, level, "deals_at_risk", consequence)
	if level == levelMaterialRisk {
		row.Because = append(row.Because, reason("material", moneyOf(expected)))
	} else if known {
		row.Because = append(row.Because, reason("below_material", moneyOf(expected)))
	}
	quiet := quietDaysOf(item)
	if quiet > 0 {
		row.Because = append(row.Because, reason("quiet_days", daysValue(quiet)))
	}
	// The close date is a deadline the customer agreed to, so it ranks like
	// one. Without this the risk lane compared on idle days alone, and a deal
	// already past its date lost to one merely quiet for longer.
	if item.DueAt != nil {
		row.Because = append(row.Because, reason("closing_soon", nil))
	}
	return ranked{
		item:         row,
		deadlineAt:   deadlineOf(item.DueAt),
		overdue:      item.Overdue != nil && *item.Overdue,
		expectedBase: expected,
		hasExpected:  known,
		waitingDays:  quiet,
		occurredAt:   occurredOf(item, asOf),
	}
}

// dealFactsOf carries the deal's own figures onto the queue row. The lane feed
// already resolved them; dropping them here would make the client read a second
// endpoint per row to draw a card this one could have completed.
func dealFactsOf(item crmcontracts.AttentionItem) *crmcontracts.WorklistDealFacts {
	if item.Deal == nil {
		return nil
	}
	facts := &crmcontracts.WorklistDealFacts{
		StageId:     item.Deal.StageId,
		OwnerId:     item.Deal.OwnerId,
		AmountMinor: item.Deal.AmountMinor,
		Currency:    item.Deal.Currency,
	}
	// The close date rides on the lane item's own due moment, and the idle
	// count on its detail. Both were already resolved; only this projection
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

// expectedRevenue is what the deal is worth times how likely it is to land.
//
// The win probability lives on the stage rather than the deal, and this feed
// does not read stages — so until that read exists the amount stands in for the
// expectation. Naming that here rather than silently multiplying by one: the
// figure is comparable between deals in one currency, which is what the
// ordering needs, and it will get more accurate rather than change meaning.
func expectedRevenue(item crmcontracts.AttentionItem) (int64, bool) {
	if item.Deal == nil || item.Deal.AmountMinor == nil {
		return 0, false
	}
	return *item.Deal.AmountMinor, true
}

func moneyOf(minor int64) *crmcontracts.WorklistValue {
	value := minor
	return &crmcontracts.WorklistValue{Kind: "money", Minor: &value}
}
