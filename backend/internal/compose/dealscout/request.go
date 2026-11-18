// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dealscout holds Deal Scout's evidence rule. It says which evidence
// qualifies a company, which draft a suggestion is written from, and what an
// opened deal may carry. compose runs the pass and opens the deals.
package dealscout

import (
	"strings"
	"time"
)

// The facts a new_opportunity cite carries under "fields". Deal Scout reads an
// explicit request as enough to open a deal on its own.
const (
	RequestKindKey      = "request_kind"
	BudgetQuoteKey      = "budget_quote"
	DecisionDateKey     = "decision_date"
	RequestKindExplicit = "explicit_request"
	RequestKindNeed     = "need"
)

// RequestFacts are a request's facts as the cited message states them, for the
// cite's "fields". body is the sender's own words. A budget the body does not
// contain, or a decision day that is no date, is dropped rather than filed.
// The budget keeps the words as quoted, so QuotedBudget reads only them.
func RequestFacts(requestKind, budgetQuote, decisionDate, body string) map[string]any {
	if requestKind == "" {
		return nil
	}
	facts := map[string]any{RequestKindKey: requestKind}
	if _, err := time.Parse(time.DateOnly, decisionDate); err == nil {
		facts[DecisionDateKey] = decisionDate
	}
	quote := strings.Join(strings.Fields(budgetQuote), " ")
	if quote != "" && strings.Contains(strings.Join(strings.Fields(body), " "), quote) {
		facts[BudgetQuoteKey] = quote
	}
	return facts
}
