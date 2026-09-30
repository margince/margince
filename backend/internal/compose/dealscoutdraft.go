// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
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
// the draft deals records. Which evidence counts was decided by the read; this
// only names the lead kind, weighs the kinds present, and takes the amount of
// the newest document whose reading stated one with its currency.
func draftSuggestion(company []scoutItem) deals.SuggestionDraft {
	draft := deals.SuggestionDraft{CompanyID: company[0].company}
	present := map[string]bool{}
	for _, it := range company {
		present[it.evidence.Kind] = true
		draft.Evidence = append(draft.Evidence, it.evidence)
		if draft.AmountMinor == nil && it.amountMinor != nil && it.currency != nil {
			draft.AmountMinor, draft.Currency = it.amountMinor, it.currency
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
