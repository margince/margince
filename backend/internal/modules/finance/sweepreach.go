// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

import "github.com/margince/margince/backend/pkg/extension/reach"

// SweepReach declares the finance columns holding a company id, so the company
// merge moves them onto the record that stays. Invoices and payments are ON
// DELETE RESTRICT. A row left on the retired company pins it, and a finance
// user reads it as money gone with the duplicate.
func SweepReach() []reach.Table {
	company := []reach.Link{{Column: "company_id", Record: reach.Company}}
	return []reach.Table{
		{Name: "finance_customer_link", Links: company},
		{Name: "finance_invoice", Links: company},
		{Name: "finance_payment", Links: company},
	}
}
