// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

import crmcontracts "github.com/margince/margince/backend/internal/contracts"

// fillOverdue carries the account warning's figures: what is past due, how
// many invoices, the oldest one's age and the highest reminder sent. The age
// and the level are absent when nothing is overdue, since neither has a value
// then.
func fillOverdue(open OpenBalance, currency string, out *crmcontracts.CompanyFinanceSummary) {
	out.Overdue = money(open.OverdueMinorBase, currency)
	count := open.OverdueCount
	out.OverdueCount = &count
	if count == 0 {
		return
	}
	oldest := open.OldestOverdueDays
	out.OldestOverdueDays = &oldest
	out.DunningLevel = open.HighestDunningLevel
}
