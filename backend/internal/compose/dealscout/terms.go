// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealscout

import (
	"time"

	"github.com/margince/margince/backend/internal/compose/dealverdict"
)

// Request is one explicit request's facts as Deal Scout cites them.
type Request struct {
	BudgetQuote, DecisionDate *string
	// Shareable is false when what the request's mail says may not reach a
	// deal. Then the mail is protected, or the switch to share it is off.
	Shareable bool
}

// Terms is what a deal's evidence states for its amount and close date.
type Terms struct {
	AmountMinor *int64
	Currency    *string
	// BudgetRead says the amount is a request's budget, which a model read.
	// CurrencyAssumed says that budget named no currency, so base was used.
	BudgetRead, CurrencyAssumed bool
	CloseDate                   *time.Time
}

// NewestRequest is the request a deal takes its terms from: the first in
// candidates, which are newest first. A nil entry is evidence that is no
// request. The draft and the opener both pick through it.
func NewestRequest(candidates []*Request) *Request {
	for _, candidate := range candidates {
		if candidate != nil {
			return candidate
		}
	}
	return nil
}

// DealTerms decides a deal's amount and close date, for the suggestion and for
// the deal it opens. A document's stated amount wins over a budget. A budget
// naming no currency is read in base. The decision day is the close date
// unless it is past. A request that is not shareable gives nothing.
func DealTerms(documentMinor *int64, documentCurrency *string, request *Request, base string, today time.Time) Terms {
	terms := Terms{}
	if documentMinor != nil && documentCurrency != nil {
		terms.AmountMinor, terms.Currency = documentMinor, documentCurrency
	}
	if request == nil || !request.Shareable {
		return terms
	}
	if request.DecisionDate != nil {
		terms.CloseDate = RequestCloseDate(*request.DecisionDate, today)
	}
	if terms.AmountMinor != nil || request.BudgetQuote == nil {
		return terms
	}
	if minor, currency, assumed, ok := dealverdict.QuotedBudget(*request.BudgetQuote, base); ok {
		terms.AmountMinor, terms.Currency = &minor, &currency
		terms.BudgetRead, terms.CurrencyAssumed = true, assumed
	}
	return terms
}

// RequestCloseDate is a request's decision day as a deal close date. It is nil
// when the day is no date or before today, since an open deal takes no past date.
func RequestCloseDate(day string, today time.Time) *time.Time {
	at, err := time.Parse(time.DateOnly, day)
	if err != nil || at.Before(today) {
		return nil
	}
	return &at
}
