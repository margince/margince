// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"path"
	"strings"

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

// draftSuggestion applies the rule to one company's evidence, newest first,
// and answers the draft when the rule fires. The rule fires on a held meeting,
// on a proposal document, or on the two signals within signalPairWindow; a
// lone signal is not enough.
func draftSuggestion(company []scoutItem) (deals.SuggestionDraft, bool) {
	var meetings, documents []scoutItem
	var opportunities, commitments []scoutItem
	for _, it := range company {
		switch {
		case it.evidence.Kind == deals.EvidenceMeeting:
			meetings = appendCapped(meetings, it)
		case it.evidence.Kind == deals.EvidenceAttachment:
			documents = appendCapped(documents, it)
		case it.signalKind == "new_opportunity":
			opportunities = append(opportunities, it)
		case it.signalKind == "commitment_made":
			commitments = append(commitments, it)
		}
	}
	pair := signalPair(opportunities, commitments)
	cited := append(append(append([]scoutItem{}, documents...), pair...), meetings...)
	if len(cited) == 0 {
		return deals.SuggestionDraft{}, false
	}
	draft := deals.SuggestionDraft{
		CompanyID:  company[0].company,
		Name:       suggestionName(company[0].companyName, documents),
		Confidence: scoutConfidence(presentWeights(documents, pair, meetings)),
	}
	for _, it := range cited {
		draft.Evidence = append(draft.Evidence, it.evidence)
	}
	for _, doc := range documents {
		if doc.amountMinor != nil && doc.currency != nil {
			draft.AmountMinor, draft.Currency = doc.amountMinor, doc.currency
			break
		}
	}
	return draft, true
}

// presentWeights lists the weight of each kind of evidence that is present.
func presentWeights(documents, pair, meetings []scoutItem) []float64 {
	var weights []float64
	for _, kind := range []struct {
		items  []scoutItem
		weight float64
	}{{documents, scoutConfidenceDocument}, {pair, scoutConfidenceSignals}, {meetings, scoutConfidenceMeeting}} {
		if len(kind.items) > 0 {
			weights = append(weights, kind.weight)
		}
	}
	return weights
}

func appendCapped(list []scoutItem, it scoutItem) []scoutItem {
	if len(list) >= dealScoutItemCap {
		return list
	}
	return append(list, it)
}

// signalPair answers the newest new_opportunity with a commitment_made within
// signalPairWindow of it, as the two items to cite, or nothing.
func signalPair(opportunities, commitments []scoutItem) []scoutItem {
	for _, opportunity := range opportunities {
		for _, commitment := range commitments {
			gap := opportunity.evidence.OccurredAt.Sub(commitment.evidence.OccurredAt)
			if gap < 0 {
				gap = -gap
			}
			if gap <= signalPairWindow {
				return []scoutItem{opportunity, commitment}
			}
		}
	}
	return nil
}

// suggestionName is the company's name, followed by the newest proposal's
// file name when one was sent: the rep sees at once which piece of work it is.
func suggestionName(company string, documents []scoutItem) string {
	if len(documents) == 0 {
		return company
	}
	stem := strings.TrimSpace(strings.TrimSuffix(documents[0].filename, path.Ext(documents[0].filename)))
	if stem == "" {
		return company
	}
	return company + " – " + stem
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
