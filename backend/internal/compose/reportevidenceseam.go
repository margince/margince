// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// search_report_evidence's engine seam: the run's record set from the report
// drawer's drill-through (reportevidence.go), searched through the search
// module's retrieval seam and nothing else. Every search call is bounded to
// the records the reader reached, so a record outside the run cannot be found
// however well it matches.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/retrieval"
)

// reportEvidenceSeam's SearchReportEvidence is the agents.ReportEvidenceSearcher.
type reportEvidenceSeam struct {
	db *database.DB
	// floor is the installation's, as on run_analytics_query: a cell a
	// contact may not open is one a model asking for them may not open either.
	floor      analyticsquery.Floor
	ranker     retrieval.Retriever
	classifier retrieval.Classifier
}

// SearchReportEvidence classifies every reached record, then ranks only the
// matched ones so the strongest citations lead.
func (s reportEvidenceSeam) SearchReportEvidence(
	ctx context.Context, q agents.ReportEvidenceQuery,
) (agents.ReportEvidence, error) {
	var cohort reportRunCohort
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		cohort, err = readReportRunCohort(ctx, tx, q.RunID, q.Cell, s.floor)
		return err
	}); err != nil {
		return agents.ReportEvidence{}, err
	}
	out := agents.ReportEvidence{
		Entity: cohort.Entity, Withheld: cohort.Withheld, Truncated: cohort.Truncated,
		Unreached: cohort.Unreached, Unestablished: cohort.Unestablished,
	}
	if cohort.Withheld || len(cohort.Reached) == 0 {
		return out, nil
	}
	verdicts, err := s.classifier.Classify(ctx, retrieval.ClassifyQuery{
		Text: q.Text, EntityType: cohort.Entity, Within: cohort.Reached,
	})
	if err != nil {
		return agents.ReportEvidence{}, err
	}
	out.Counterexamples, out.Abstentions = verdicts.Unmatched, verdicts.Unjudged
	if len(verdicts.Matched) == 0 {
		return out, nil
	}
	ranked, err := s.ranker.Search(ctx, retrieval.Query{
		Text: q.Text, EntityTypes: []datasource.EntityType{cohort.Entity},
		Limit: q.Limit, Within: verdicts.Matched,
	})
	if err != nil {
		return agents.ReportEvidence{}, err
	}
	out.Citations = rankedFirst(cohort.Entity, verdicts.Matched, ranked.Hits)
	out.SemanticRanking = ranked.SemanticRanking
	return out, nil
}

// rankedFirst orders the matched records: the ranking's page first, with the
// excerpts it ranked on, then every other match. A ranked hit the classifier
// did not match is not evidence and is left out.
func rankedFirst(entity datasource.EntityType, matched []ids.UUID, ranked []retrieval.Hit) []retrieval.Hit {
	isMatched := make(map[ids.UUID]bool, len(matched))
	for _, id := range matched {
		isMatched[id] = true
	}
	out := make([]retrieval.Hit, 0, len(matched))
	placed := map[ids.UUID]bool{}
	for _, hit := range ranked {
		if isMatched[hit.Ref.ID] && !placed[hit.Ref.ID] {
			out = append(out, hit)
			placed[hit.Ref.ID] = true
		}
	}
	for _, id := range matched {
		if !placed[id] {
			out = append(out, retrieval.Hit{Ref: datasource.EntityRef{Type: entity, ID: id}})
		}
	}
	return out
}
