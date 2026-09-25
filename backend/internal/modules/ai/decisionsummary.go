// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// DecisionSummary is how often one task consulted the decision lane over a
// window, how often its answer stood, and why the rest went to the ladder.
// Every count is of logical calls: ai_usage counts attempts, so a decision
// that fell back would count twice there and no rate could be read from it.
type DecisionSummary struct {
	Task    string
	Asked   int
	Decided int
	// Fallbacks is keyed by the decision attempt reason the ladder carried.
	// Never nil, so a task whose every answer stood reads as "none fell back".
	Fallbacks map[string]int
}

// decisionSummaryGroup is one (task, fallback reason) row of the summary
// query; reason is "" for the logical calls that carried no decision reason.
type decisionSummaryGroup struct {
	task    string
	reason  string
	calls   int
	decided int
}

// decisionSummaryQuery reads one row per (task, fallback reason) over the
// logical calls that consulted the decision lane: a decision row was written,
// or the ladder's attempt carries a decision reason — the second alone when the
// lane refused before sending anything. Grouping by logical call first is what
// counts a decision that errored, and the ladder that says so, once.
const decisionSummaryQuery = `
	WITH consulted AS (
		SELECT task,
		       bool_or(kind = $3 AND is_terminal) AS decided,
		       min(attempt_reason) FILTER (WHERE kind <> $3 AND attempt_reason = ANY($4)) AS reason
		FROM ai_call
		WHERE occurred_at >= $1 AND occurred_at < $2
		GROUP BY task, logical_call_id
		HAVING bool_or(kind = $3) OR bool_or(attempt_reason = ANY($4))
	)
	SELECT task, COALESCE(reason, ''), count(*), count(*) FILTER (WHERE decided)
	FROM consulted
	GROUP BY task, reason
	ORDER BY task, reason`

// DecisionSummaries reads the [from, to] usage window's decision consultations
// per task, ordered by task. It is the usage read's companion and admitted by
// the same grant.
func (s *CallReadStore) DecisionSummaries(ctx context.Context, from, to time.Time) ([]DecisionSummary, error) {
	if err := auth.Require(ctx, "ai_diagnostics", principal.ActionRead); err != nil {
		return nil, err
	}
	var groups []decisionSummaryGroup
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, decisionSummaryQuery,
			from, usageCallWindowEnd(to), callKindDecision, decisionAttemptReasons)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var g decisionSummaryGroup
			if err := rows.Scan(&g.task, &g.reason, &g.calls, &g.decided); err != nil {
				return err
			}
			groups = append(groups, g)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("ai: decision summary: %w", err)
	}
	return foldDecisionSummaries(groups), nil
}

// foldDecisionSummaries folds task-ordered groups into one summary per task.
func foldDecisionSummaries(groups []decisionSummaryGroup) []DecisionSummary {
	var out []DecisionSummary
	for _, g := range groups {
		if len(out) == 0 || out[len(out)-1].Task != g.task {
			out = append(out, DecisionSummary{Task: g.task, Fallbacks: map[string]int{}})
		}
		line := &out[len(out)-1]
		line.Asked += g.calls
		line.Decided += g.decided
		if g.reason != "" {
			line.Fallbacks[g.reason] += g.calls
		}
	}
	return out
}
