// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/retrieval"
)

// activityWithContent is an activity as the provider reads it back, its content
// either still the caller's to read or withheld from them since the search.
func activityWithContent(state string) datasource.Record {
	return datasource.Record{
		Ref:    datasource.EntityRef{Type: datasource.EntityActivity},
		Fields: json.RawMessage(`{"captured_by":"human:x","content_state":"` + state + `"}`),
	}
}

func pricingHit(id ids.UUID) retrieval.Hit {
	return retrieval.Hit{
		Ref: datasource.EntityRef{Type: datasource.EntityActivity, ID: id}, Score: 0.8,
		Evidence: []retrieval.Evidence{{Source: "activity:" + id.String(), Snippet: "pushed back on pricing"}},
	}
}

// An email restricted between the search and the read-back: the search judged
// its body, the read-back says the body is withheld. Its excerpt and verdict go,
// the tally loses it, and no share is stated over a set that just changed.
func TestAReportEvidenceCitationWithheldAtReadBackTakesNothingWithIt(t *testing.T) {
	kept, restricted := ids.NewV7(), ids.NewV7()
	provider := &queryProbeProvider{records: map[ids.UUID]datasource.Record{
		kept:       activityWithContent("available"),
		restricted: activityWithContent("withheld"),
	}}
	tool := searchReportEvidence{
		hydrator: searchContext{p: provider},
		search: func(context.Context, ReportEvidenceQuery) (ReportEvidence, error) {
			return ReportEvidence{
				Entity:    datasource.EntityActivity,
				Citations: []retrieval.Hit{pricingHit(kept), pricingHit(restricted)},
			}, nil
		},
	}
	raw, err := tool.Handle(t.Context(), json.RawMessage(`{"run_id":"`+ids.NewV7().String()+`","query":"pricing"}`))
	if err != nil {
		t.Fatal(err)
	}
	var result SearchReportEvidenceResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	for _, hit := range result.Citations {
		if hit.Record.ID == restricted {
			t.Fatalf("a citation whose content is withheld was served with its excerpt: %+v", hit)
		}
	}
	if len(result.Citations) != 1 || result.Tally.Matched != 1 {
		t.Errorf("citations %d, tally %+v: want the one readable match counted alone", len(result.Citations), result.Tally)
	}
	if result.Prevalence != nil || result.Coverage != CoveragePartialDegraded {
		t.Errorf("coverage %q, prevalence %+v stated over a record lost at read-back", result.Coverage, result.Prevalence)
	}
}

// The same read-back guards search_context: a hit ranked on content the caller
// may no longer read is dropped, excerpt and all.
func TestASearchContextHitWithheldAtReadBackIsDropped(t *testing.T) {
	kept, restricted := ids.NewV7(), ids.NewV7()
	provider := &queryProbeProvider{records: map[ids.UUID]datasource.Record{
		kept:       activityWithContent("available"),
		restricted: activityWithContent("withheld"),
	}}
	result := handleContextSearch(t, provider, retrieval.Result{
		SemanticRanking: true, Hits: []retrieval.Hit{pricingHit(kept), pricingHit(restricted)},
	})
	for _, hit := range result.Hits {
		if hit.Record.ID == restricted {
			t.Fatalf("a hit whose content is withheld was served with its excerpt: %+v", hit)
		}
	}
	if len(result.Hits) != 1 || result.Coverage != CoveragePartialDegraded {
		t.Errorf("hits %d, coverage %q: want the readable hit alone, the drop reported", len(result.Hits), result.Coverage)
	}
}

// An abstention is read back like a citation: one the caller can no longer
// read is not served, and is not counted.
func TestAnAbstentionUnreadableAtReadBackIsNotServed(t *testing.T) {
	readable, gone := ids.NewV7(), ids.NewV7()
	provider := &queryProbeProvider{records: map[ids.UUID]datasource.Record{
		readable: activityWithContent("available"),
	}}
	tool := searchReportEvidence{
		hydrator: searchContext{p: provider},
		search: func(context.Context, ReportEvidenceQuery) (ReportEvidence, error) {
			return ReportEvidence{Entity: datasource.EntityActivity, Abstentions: []ids.UUID{readable, gone}}, nil
		},
	}
	raw, err := tool.Handle(t.Context(), json.RawMessage(`{"run_id":"`+ids.NewV7().String()+`","query":"pricing"}`))
	if err != nil {
		t.Fatal(err)
	}
	var result SearchReportEvidenceResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Abstentions) != 1 || result.Abstentions[0].ID != readable || result.Tally.Unjudged != 1 {
		t.Errorf("abstentions %+v, tally %+v: want the readable one alone", result.Abstentions, result.Tally)
	}
}
