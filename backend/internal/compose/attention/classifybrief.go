// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The overnight brief's own classifier — its own file for the reason
// classifyrisk.go and classifydecision.go already split out of classify.go:
// classify.go was already at the 500-line file cap this repo holds.

import (
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// classifyBriefItem: what the overnight run put at the top of the day. It is a
// suggestion about where to start rather than something waiting on the reader,
// so it sits with agreed work — but it belongs ON the queue, because a lane the
// ranking never sees is a lane the reader was told to read separately, which is
// the arrangement this endpoint exists to end.
//
// The date is a deal's expected close date (applyDealFigures, dealfacts.go) —
// the same fact classifyRisk's "deals_at_risk" rows carry, so this reads it
// the way that sibling does rather than the way classifyTask reads a task's
// due date: closing_soon whenever a date is set, never due_today, because a
// close date three months out is a forecast and not work owed for today.
//
// Overdue is never recomputed here — it arrives on item.Overdue from the SAME
// deal-figures read that supplied DueAt (applyDealFigures, dealfacts.go),
// stating deals.CloseIsOverdue's own verdict, the identical rule the sibling
// "deals_at_risk" row for the same deal is judged by. base() already carries
// item.Overdue onto row.Overdue; there is nothing left to set here.
//
// Priced the same way classifyRisk prices its own "deals_at_risk" row for the
// identical deal, through the one shared priceDealsAtRiskRow — money is a
// property of the DEAL, not of which lane put it on the queue today.
func classifyBriefItem(item crmcontracts.AttentionItem, asOf time.Time, money dayMoney) ranked {
	row := base(item, levelAgreed, "deals_at_risk", "deal_drifts")
	if item.DueAt != nil {
		row.Because = append(row.Because, reason("closing_soon", nil))
	}
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
