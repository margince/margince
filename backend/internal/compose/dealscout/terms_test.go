// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealscout

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/deals"
)

func TestRequestFactsKeepOnlyWhatTheMessageStates(t *testing.T) {
	t.Parallel()
	body := "Please send a quotation.\nOur budget is 10,000 per year."
	facts := RequestFacts(RequestKindExplicit, "our budget  is 10,000", "2026-13-40", body)
	if _, kept := facts[BudgetQuoteKey]; kept {
		t.Errorf("a budget the body does not contain was kept: %v", facts)
	}
	if _, kept := facts[DecisionDateKey]; kept {
		t.Errorf("a decision day that is no date was kept: %v", facts)
	}
	facts = RequestFacts(RequestKindExplicit, "Our budget  is 10,000", "2026-12-01", body)
	if facts[BudgetQuoteKey] != "Our budget is 10,000" || facts[DecisionDateKey] != "2026-12-01" {
		t.Errorf("facts = %v, want the quoted budget words and the day", facts)
	}
	if facts := RequestFacts("", "Our budget is 10,000", "2026-12-01", body); facts != nil {
		t.Errorf("an event that is no request got facts %v", facts)
	}
}

func TestAPastDecisionDayGivesNoCloseDate(t *testing.T) {
	t.Parallel()
	today := time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)
	if got := RequestCloseDate("2026-11-18", today); got != nil {
		t.Errorf("a past decision day gave close date %v", got)
	}
	if got := RequestCloseDate("2026-11-19", today); got == nil || !got.Equal(today) {
		t.Errorf("today's decision day gave %v, want today", got)
	}
}

func TestADocumentAmountWinsOverABudgetAndAProtectedRequestGivesNothing(t *testing.T) {
	t.Parallel()
	today := time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)
	budget, day := "budget 10,000", "2026-12-01"
	minor, eur := int64(1_250_000), "EUR"
	documented := []Item{
		{evidence: deals.SuggestionEvidence{Kind: deals.EvidenceSignal}, request: &Request{BudgetQuote: &budget, DecisionDate: &day, Shareable: true}},
		{evidence: deals.SuggestionEvidence{Kind: deals.EvidenceAttachment}, amountMinor: &minor, currency: &eur},
	}
	draft := Draft(documented, "USD", today)
	if draft.AmountMinor == nil || *draft.AmountMinor != minor || *draft.Currency != eur || draft.CloseDate == nil {
		t.Errorf("draft = %v %v %v, want the document's 12,500 EUR and the decision day", draft.AmountMinor, draft.Currency, draft.CloseDate)
	}
	protected := []Item{{evidence: deals.SuggestionEvidence{Kind: deals.EvidenceSignal},
		request: &Request{BudgetQuote: &budget, DecisionDate: &day}}}
	if draft := Draft(protected, "USD", today); draft.AmountMinor != nil || draft.CloseDate != nil {
		t.Errorf("a protected request drafted %v / %v, want neither", draft.AmountMinor, draft.CloseDate)
	}
	if terms := DealTerms(nil, nil, protected[0].request, "USD", today); terms.BudgetRead || terms.CloseDate != nil {
		t.Errorf("the opener's terms from a protected request = %+v, want none", terms)
	}
}
