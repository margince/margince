// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// Admitted brief suggestions share current deal observations with the risk lane.
func classifyBriefItem(item crmcontracts.AttentionItem, asOf time.Time, money dayMoney) ranked {
	row := base(item, levelAgreed, "deals_at_risk", briefConsequence(item))
	row.Because = append(row.Because, dealInterventionReasons(item, asOf)...)
	expected, known := expectedRevenue(item, money)
	priceDealsAtRiskRow(&row, expected, known, money)
	return ranked{
		item: row,
		// Deferred to the deal for the reason the at-risk lane defers: the owner
		// arrives with the facts pass, after this runs.
		ownerRef:         deferredToTheDeal(),
		deadlineAt:       deadlineOf(item.DueAt),
		overdue:          item.Overdue != nil && *item.Overdue,
		expectedBase:     expected,
		hasExpected:      known,
		expectedCurrency: money.base,
		occurredAt:       occurredOf(item, asOf),
	}
}

// A ranking signal alone must not invent a failure for a healthy deal.
func briefConsequence(item crmcontracts.AttentionItem) crmcontracts.WorklistItemConsequence {
	if item.Kind == nil {
		return valueNone
	}
	switch *item.Kind {
	case "closing_soon":
		// The date is the fact: left alone, this one goes past its close.
		return "deal_slips_past_close"
	case "opportunity", "moved":
		return valueNone
	case "stalled":
		return "deal_drifts"
	default:
		return valueNone
	}
}
