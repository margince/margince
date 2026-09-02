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
// The date is a deal's expected close date (applyDealFigures, dealfacts.go),
// not a deadline the reader owes — so unlike classifyTask this never claims
// due_today for it: a close date three months out is not due today, and this
// lane runs no query narrowing it to today the way the task queue's
// OpenAndDueBy does. Overdue is the one fact worth stamping, because a close
// date that has already passed is exactly the "deal drifts" this row exists
// to say.
func classifyBriefItem(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	row := base(item, levelAgreed, "deals_at_risk", "deal_drifts")
	stampDeadline(&row, item.DueAt, asOf)
	if overdueAt(item.DueAt, asOf) {
		row.Because = append(row.Because, reason("overdue", nil))
	}
	return ranked{
		item:       row,
		deadlineAt: deadlineOf(item.DueAt),
		overdue:    overdueAt(item.DueAt, asOf),
		occurredAt: occurredOf(item, asOf),
	}
}
