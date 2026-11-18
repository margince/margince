// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealscout

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

// Draft turns one chosen company's cited evidence, newest first, into the
// draft deals records. The read decided which evidence counts. This names the
// lead kind and weighs the kinds present. DealTerms decides the amount and the
// close date from the newest document's amount and the newest request.
func Draft(company []Item, base string, today time.Time) deals.SuggestionDraft {
	draft := deals.SuggestionDraft{CompanyID: company[0].company}
	present := map[string]bool{}
	var documentMinor *int64
	var documentCurrency *string
	requests := make([]*Request, 0, len(company))
	for _, it := range company {
		present[it.evidence.Kind] = true
		draft.Evidence = append(draft.Evidence, it.evidence)
		if documentMinor == nil && it.amountMinor != nil && it.currency != nil {
			documentMinor, documentCurrency = it.amountMinor, it.currency
		}
		requests = append(requests, it.request)
	}
	terms := DealTerms(documentMinor, documentCurrency, NewestRequest(requests), base, today)
	draft.AmountMinor, draft.Currency, draft.CloseDate = terms.AmountMinor, terms.Currency, terms.CloseDate
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

// Item is one cited piece of evidence about one chosen company.
type Item struct {
	company     ids.UUID
	evidence    deals.SuggestionEvidence
	amountMinor *int64
	currency    *string
	// request is set when the evidence is an explicit request.
	request *Request
}

// ReadEvidence answers the evidence each suggestion will cite, for at most
// companyCap companies. One statement decides it all in the database. It picks
// the evidence that counts, the companies it qualifies and those the cap keeps.
// For the kept companies' documents it also reads what their readings stated.
func ReadEvidence(ctx context.Context, tx pgx.Tx, since, now time.Time, companyCap int) ([]Item, error) {
	share, err := activities.SharesPrivateRequestsTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	query, args := EvidenceSQL(since, now, companyCap, share)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("deal scout: reading the evidence: %w", err)
	}
	items, err := pgx.CollectRows(rows, scanItem)
	if err != nil {
		return nil, fmt.Errorf("deal scout: scanning the evidence: %w", err)
	}
	return items, nil
}

func scanItem(row pgx.CollectableRow) (Item, error) {
	var it Item
	var ref ids.UUID
	var amount, currency *string
	var asked bool
	var request Request
	if err := row.Scan(&it.company, &it.evidence.Kind, &ref, &it.evidence.OccurredAt, &amount, &currency,
		&asked, &request.DecisionDate, &request.BudgetQuote, &request.Shareable); err != nil {
		return Item{}, err
	}
	if asked {
		it.request = &request
	}
	switch it.evidence.Kind {
	case deals.EvidenceMeeting:
		it.evidence.ActivityID = &ref
	case deals.EvidenceSignal:
		it.evidence.SignalID = &ref
	default:
		it.evidence.AttachmentID = &ref
	}
	if amount != nil && currency != nil {
		if minor, err := strconv.ParseInt(*amount, 10, 64); err == nil {
			it.amountMinor, it.currency = &minor, currency
		}
	}
	return it, nil
}

// ByCompany groups the rows, which arrive ordered by company.
func ByCompany(items []Item) [][]Item {
	var out [][]Item
	for _, it := range items {
		if n := len(out); n > 0 && out[n-1][0].company == it.company {
			out[n-1] = append(out[n-1], it)
			continue
		}
		out = append(out, []Item{it})
	}
	return out
}
