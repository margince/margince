// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// search_report_evidence's engine seam: a saved run's records as this reader
// reaches them, through the drill-through the report drawer opens
// (reportRunExplain, ExplainAnalyticsCell), searched through the search
// module's retrieval seam and nothing else. Every search call is bounded to
// those records, so a record outside the run cannot be found however well it
// matches. Nothing here reads a row the reader cannot, so nothing in the
// answer can depend on one.

import (
	"context"
	"fmt"

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
	var cohort AnalyticsExplanation
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		explain, err := reportRunExplain(ctx, tx, q.RunID, q.Cell)
		if err != nil {
			return err
		}
		cohort, err = ExplainAnalyticsCell(ctx, tx, explain, s.floor)
		return err
	}); err != nil {
		return agents.ReportEvidence{}, err
	}
	out := agents.ReportEvidence{
		Entity: cohort.Entity, Withheld: cohort.Withheld,
		PartlyWithheld: cohort.PartlyWithheld, Truncated: cohort.Truncated,
	}
	reached, err := explainedIDs(cohort)
	if err != nil {
		return agents.ReportEvidence{}, err
	}
	if cohort.Withheld || len(reached) == 0 {
		return out, nil
	}
	verdicts, err := s.classifier.Classify(ctx, retrieval.ClassifyQuery{
		Text: q.Text, EntityType: cohort.Entity, Within: reached,
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

// explainedIDs reads the record id off each explained row: the drill-through's
// first column, rendered as a uuid string.
func explainedIDs(out AnalyticsExplanation) ([]ids.UUID, error) {
	found := make([]ids.UUID, 0, len(out.Rows))
	for _, row := range out.Rows {
		raw, ok := row["id"].(string)
		if !ok {
			return nil, fmt.Errorf("compose: an explained row carries no record id")
		}
		id, err := ids.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("compose: an explained row's record id: %w", err)
		}
		found = append(found, id)
	}
	return found, nil
}
