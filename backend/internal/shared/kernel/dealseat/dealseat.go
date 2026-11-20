// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dealseat is what "this contact sits on the deal's buying committee
// now" means, spelled once for every module that reads a deal_stakeholder edge.
//
// A reader that ignores the End date keeps an ended stakeholder on the
// committee. A reader that tests `ended_at IS NULL` ends a seat before its
// End date. Both shipped, and the deal page and the contact
// page then disagreed about one edge.
//
// Held by: TestEveryDealSeatCurrencyTestUsesTheOneDefinition
// (backend/gates/dealseatcurrency_test.go).
package dealseat

import "github.com/margince/margince/backend/internal/shared/kernel/occurredday"

// Kind is the relationship kind of a seat on a deal.
const Kind = "deal_stakeholder"

// CurrentSQL is a live deal_stakeholder edge whose End day has not arrived.
// An End date in the future still counts until that day. An End date of today
// has already ended, the rule employment.IsCurrentSQL keeps for a job. Today is
// the installation's day on the database clock, so every reader in one
// transaction asks it of the same day. alias is the relationship table's
// compile-time alias, or "" when the statement does not alias it.
func CurrentSQL(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	return "(" + prefix + "kind = '" + Kind + "' AND " + prefix + "archived_at IS NULL AND (" +
		prefix + "ended_at IS NULL OR " + prefix + "ended_at > " + occurredday.TodaySQL("now()") + "))"
}
