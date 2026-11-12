// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salesprogress

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// compared fills completion and delta, which only a complete goal over a
// complete amount supports. A zero goal has a delta and no completion.
func compared(closed crmcontracts.SalesProgressAmount, goal crmcontracts.SalesProgressGoal) (completion *float64, delta *int64) {
	if closed.Status != crmcontracts.SalesProgressAmountStatusOk || closed.AmountMinor == nil || goal.AmountMinor == nil || (goal.Status != crmcontracts.SalesProgressGoalStatusSet && goal.Status != crmcontracts.SalesProgressGoalStatusZero) {
		return nil, nil
	}
	difference := *closed.AmountMinor - *goal.AmountMinor
	delta = &difference
	if goal.Status == crmcontracts.SalesProgressGoalStatusSet {
		ratio := float64(*closed.AmountMinor) / float64(*goal.AmountMinor)
		completion = &ratio
	}
	return completion, delta
}
