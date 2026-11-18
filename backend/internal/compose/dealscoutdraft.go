// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"time"

	"github.com/margince/margince/backend/internal/compose/dealverdict"
	"github.com/margince/margince/backend/internal/modules/deals"
)

// The confidence each kind of evidence carries on its own. A sent proposal is
// the strongest: somebody has already priced the work. Each further kind of
// evidence adds to the strongest, up to scoutConfidenceCeiling.
const (
	scoutConfidenceDocument = 0.8
	scoutConfidenceSignals  = 0.7
	scoutConfidenceMeeting  = 0.6
	scoutConfidenceCorrob   = 0.1
	scoutConfidenceCeiling  = 0.95
)

// draftSuggestion turns one chosen company's cited evidence, newest first, into
// the draft deals records. The read decided which evidence counts. This names
// the lead kind and weighs the kinds present. The amount is the newest
// document's stated amount with its currency. Without one, the newest explicit
// request gives the budget, read in base when it names no currency. That
// request also gives the decision day.
func draftSuggestion(company []scoutItem, base string, today time.Time) deals.SuggestionDraft {
	draft := deals.SuggestionDraft{CompanyID: company[0].company}
	present := map[string]bool{}
	for _, it := range company {
		present[it.evidence.Kind] = true
		draft.Evidence = append(draft.Evidence, it.evidence)
		if draft.AmountMinor == nil && it.amountMinor != nil && it.currency != nil {
			draft.AmountMinor, draft.Currency = it.amountMinor, it.currency
		}
	}
	for _, it := range company {
		if draft.CloseDate == nil && it.decisionDate != nil {
			draft.CloseDate = requestCloseDate(*it.decisionDate, today)
		}
		if draft.AmountMinor == nil && it.budgetQuote != nil {
			if minor, currency, _, ok := dealverdict.QuotedBudget(*it.budgetQuote, base); ok {
				draft.AmountMinor, draft.Currency = &minor, &currency
			}
		}
	}
	var weights []float64
	for _, kind := range []struct {
		evidence, hint string
		weight         float64
	}{
		{deals.EvidenceAttachment, deals.HintProposalSent, scoutConfidenceDocument},
		{deals.EvidenceSignal, deals.HintOpportunitySignaled, scoutConfidenceSignals},
		{deals.EvidenceMeeting, deals.HintMeetingHeld, scoutConfidenceMeeting},
	} {
		if !present[kind.evidence] {
			continue
		}
		if draft.NameHint == "" {
			draft.NameHint = kind.hint
		}
		weights = append(weights, kind.weight)
	}
	draft.Confidence = scoutConfidence(weights)
	return draft
}

// scoutConfidence is the strongest kind's weight, raised for each further kind
// that corroborates it.
func scoutConfidence(weights []float64) float64 {
	best := 0.0
	for _, weight := range weights {
		best = max(best, weight)
	}
	return min(best+float64(len(weights)-1)*scoutConfidenceCorrob, scoutConfidenceCeiling)
}

// requestCloseDate is a request's decision day as a deal close date. It is nil
// when the day is no date or before today, since an open deal takes no past date.
func requestCloseDate(day string, today time.Time) *time.Time {
	at, err := time.Parse(time.DateOnly, day)
	if err != nil || at.Before(today) {
		return nil
	}
	return &at
}
