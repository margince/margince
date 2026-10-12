// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companyscan

// What a reader is SERVED, as against what was read: the two writers' advice
// folded into one list, the stored half rechecked against the grants it has
// outlived, capped to what the page draws, and enriched per reader.
//
// Its own file beside the lifecycle it is called from, because the recheck is
// most of it — retraction next door asks the existence question and withholding
// asks the content one, and both act HERE, on the way out, rather than where
// the findings were written.

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/briefevidence"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// wire merges the rules' live advice with the stored findings, applies the
// reader's dismissals to the model's rows, caps, and states where the read
// stands.
func (s *Service) wire(
	ctx context.Context, companyID ids.CompanyID, stored *row, stale bool,
) (crmcontracts.CompanyScan, error) {
	rules, err := s.advice.UndismissedAdvice(ctx, companyID)
	if err != nil {
		return crmcontracts.CompanyScan{}, err
	}
	var read []crmcontracts.Company360Suggestion
	if stored != nil && len(stored.Findings) > 0 {
		read, err = s.advice.KeepUndismissed(ctx, companyID, stored.Findings)
		if err != nil {
			return crmcontracts.CompanyScan{}, err
		}
	}
	merged := merge(rules, read)
	// A STORED finding outlives the record it was written from, so what it
	// cites is asked about before it is enriched. Retraction runs BEFORE the
	// cap too, so a finding quoting an archived message does not hold a slot
	// against a live one — capping first would report the live row as "dropped
	// by the cap" and show the retracted one in its place.
	standing, err := s.standingCitations(ctx, merged)
	if err != nil {
		return crmcontracts.CompanyScan{}, err
	}
	findings, dropped := applyCap(keepCited(merged, standing))
	// And the CONTENT question, which retraction deliberately does not answer.
	// A stored finding carries the cited message's subject and a verbatim
	// extract of its body, so an audience narrowed after the write leaves those
	// words on the card — while the summary read below correctly refuses the
	// reader the live row they came from.
	if err := s.withholdQuotedWords(ctx, findings); err != nil {
		return crmcontracts.CompanyScan{}, err
	}
	// Once, over the merged list rather than in either writer: the rules' rows
	// and the stored ones cite the same account's conversations, and enriching
	// each side would read the same message twice and let one copy carry a
	// summary the other lacks.
	if err := briefevidence.Attach(ctx, s.emailRows, briefevidence.FromSuggestions(findings)); err != nil {
		return crmcontracts.CompanyScan{}, err
	}
	out := crmcontracts.CompanyScan{
		CompanyId:       openapi_types.UUID(companyID.UUID),
		State:           crmcontracts.CompanyScanStateNever,
		Findings:        findings,
		FindingsDropped: dropped,
	}
	if stored == nil {
		return out, nil
	}
	r := *stored
	out.State = crmcontracts.CompanyScanState(r.Status)
	out.GeneratedAt = r.GeneratedAt
	out.DegradeReason = r.DegradeReason
	out.ResumesAt = r.NextAttemptAt
	if r.GeneratedBy != nil {
		by := crmcontracts.WrittenBy(*r.GeneratedBy)
		out.GeneratedBy = &by
	}
	if stale {
		out.Stale = &stale
	}
	if r.ReadExchanges != nil && r.ReadDeals != nil {
		out.Read = &struct {
			Deals     int `json:"deals"`
			Exchanges int `json:"exchanges"`
		}{Deals: *r.ReadDeals, Exchanges: *r.ReadExchanges}
	}
	return out, nil
}

// merge folds both writers' advice into the list the page draws: the rules'
// rows first in their own order, then the model's in the order it gave them,
// one row per fingerprint. The cap is applied separately, after retraction.
func merge(rules, read []crmcontracts.Company360Suggestion) []crmcontracts.Company360Suggestion {
	seen := map[string]bool{}
	merged := make([]crmcontracts.Company360Suggestion, 0, len(rules)+len(read))
	for _, suggestion := range append(append([]crmcontracts.Company360Suggestion{}, rules...), read...) {
		if seen[suggestion.Fingerprint] {
			continue
		}
		seen[suggestion.Fingerprint] = true
		merged = append(merged, suggestion)
	}
	return merged
}
