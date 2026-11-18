// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dealscout holds Deal Scout's evidence rule. It says which evidence
// qualifies a company, which draft a suggestion is written from, and what an
// opened deal may carry. compose runs the pass and opens the deals.
package dealscout

// The facts a new_opportunity cite carries under "fields". Deal Scout reads an
// explicit request as enough to open a deal on its own.
const (
	RequestKindKey      = "request_kind"
	BudgetQuoteKey      = "budget_quote"
	DecisionDateKey     = "decision_date"
	RequestKindExplicit = "explicit_request"
	RequestKindNeed     = "need"
)
