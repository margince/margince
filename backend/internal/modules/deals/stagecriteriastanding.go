// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Reading a stage's exit criteria with what the evidence ledger says about each.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ProgressionCriterion is one criterion as DecideStageMove judges it, plus the
// rows a reader can click through to.
type ProgressionCriterion struct {
	Version int64
	CriterionFact
	CriterionID ids.ExitCriterionID
	Label       string
	EvidenceIDs []ids.UUID
	// Sources are the records each claim was read from (a contract, a message).
	// The evidence row's own id points at the ledger entry, not the material.
	Sources []EvidenceSource
}

// EvidenceSource is one claim's pointer back to the record it was read from,
// with the fragment it rests on. Missing is true when the reader cannot open
// that record, deleted or hidden, so nothing cites a pointer that opens nothing.
type EvidenceSource struct {
	SourceType string
	SourceID   ids.UUID
	Snippet    string
	Missing    bool
}

// readProgressionCriteria answers the stage's live criteria and what the
// ledger says about each.
//
// A criterion with no standing evidence is not met. Refuted evidence does not
// count, because a human said it was wrong. Where several rows settle one
// criterion, a buyer-authored claim beats a seller's, then the newest wins.
func (s *Store) readProgressionCriteria(
	ctx context.Context, tx pgx.Tx, stageID ids.StageID, dealID ids.DealID,
) ([]ProgressionCriterion, error) {
	args := []any{stageID, dealID}
	arg := func(v any) int { args = append(args, v); return len(args) }
	missing, err := s.sourceMissingSQL(ctx, arg)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, storekit.SQLf(`
		SELECT c.id, c.key, c.label, c.required, c.kind, c.version,
		       coalesce(e.met, false), coalesce(e.author_side, ''),
		       coalesce(e.commitment, ''), e.confidence,
		       coalesce(e.contradicted, false), coalesce(e.ids, '{}'),
		       coalesce(e.source_types, '{}'), coalesce(e.source_ids, '{}'),
		       coalesce(e.snippets, '{}'), coalesce(e.sources_missing, '{}')
		  FROM stage_exit_criterion c
		  LEFT JOIN LATERAL (
		      SELECT bool_or(v.met) AS met,
		             (array_agg(v.author_side ORDER BY v.rank, v.observed_at DESC))[1] AS author_side,
		             (array_agg(v.commitment ORDER BY v.rank, v.observed_at DESC))[1] AS commitment,
		             (array_agg(v.confidence ORDER BY v.rank, v.observed_at DESC))[1] AS confidence,
		             -- ANY row's contradiction, not the winner's. A newer
		             -- uncontradicted claim can out-rank an older contradicted
		             -- one on the same criterion, and picking only the winner
		             -- would drop the disagreement exactly when a reader most
		             -- needs telling that one exists.
		             bool_or(v.contradicted_by IS NOT NULL) AS contradicted,
		             -- ORDERED, because this list is hashed. The card's
		             -- diff_hash is what binds a proposal to its payload and
		             -- what tells one proposal from another; an unordered
		             -- aggregate lets two readings of the SAME evidence hash
		             -- differently, so a re-read would supersede its own card
		             -- with an identical one and the inbox would churn.
		             array_agg(v.id ORDER BY v.id) AS ids,
		             -- The records the claims were read FROM, so the card
		             -- cites material a reviewer can open rather than the
		             -- ledger row that records the reading.
		             array_agg(v.source_type ORDER BY v.id) AS source_types,
		             array_agg(v.source_id ORDER BY v.id) AS source_ids,
		             array_agg(coalesce(v.snippet, '') ORDER BY v.id) AS snippets,
		             array_agg(v.source_missing ORDER BY v.id) AS sources_missing
		        FROM (
		            SELECT ev.*,
		                   CASE WHEN ev.author_side = 'buyer' THEN 0 ELSE 1 END AS rank,
		                   %s AS source_missing
		              FROM deal_stage_evidence ev
		             WHERE ev.criterion_id = c.id AND ev.deal_id = $2
		               AND ev.refuted_at IS NULL AND ev.met
		        ) v
		  ) e ON true
		 WHERE c.stage_id = $1 AND c.archived_at IS NULL
		 ORDER BY c."position"`, missing), args...)
	if err != nil {
		return nil, fmt.Errorf("read the stage's criteria and their evidence: %w", err)
	}
	defer rows.Close()

	var out []ProgressionCriterion
	for rows.Next() {
		var c ProgressionCriterion
		var kind, authorSide, commitment string
		var sourceTypes, snippets []string
		var sourceIDs []ids.UUID
		var sourcesMissing []bool
		if err := rows.Scan(&c.CriterionID, &c.Key, &c.Label, &c.Required, &kind, &c.Version,
			&c.Met, &authorSide, &commitment, &c.Confidence,
			&c.Contradicted, &c.EvidenceIDs,
			&sourceTypes, &sourceIDs, &snippets, &sourcesMissing); err != nil {
			return nil, fmt.Errorf("scan a criterion's standing: %w", err)
		}
		c.Sources = evidenceSources(sourceTypes, sourceIDs, snippets, sourcesMissing)
		c.Kind = CriterionKind(kind)
		c.AuthorSide = AuthorSide(authorSide)
		c.Commitment = commitment
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the stage's criteria: %w", err)
	}
	return out, nil
}

// evidenceSources zips the four parallel aggregates into one list.
//
// All four are aggregated over the same rows in the same order, so they are
// the same length. A short one is still read only as far as it goes. A card
// citing fewer sources is a smaller answer, and a panic stops the proposer.
func evidenceSources(types []string, ids []ids.UUID, snippets []string, missing []bool) []EvidenceSource {
	n := min(len(types), len(ids), len(snippets), len(missing))
	if n == 0 {
		return nil
	}
	out := make([]EvidenceSource, 0, n)
	for i := range n {
		out = append(out, EvidenceSource{
			SourceType: types[i], SourceID: ids[i], Snippet: snippets[i], Missing: missing[i],
		})
	}
	return out
}
