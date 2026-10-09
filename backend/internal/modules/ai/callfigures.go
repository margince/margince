// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The call figures an admin tunes requests by: how many calls a provider,
// model, host, tier or task served, how many failed or timed out, how long
// they took and what they cost, and — per task — which step of its route
// answered.
//
// Every figure reads ai_call through the predicates health.go reads it by, so
// a tier the health dot shows failing is failing here too.

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CallStatsGroup is the column the figures are grouped by.
type CallStatsGroup string

// The groupings a reader may ask for. served_provider is the host a broker
// routed the call to, which only a broker reports.
const (
	GroupByProvider       CallStatsGroup = "provider"
	GroupByModel          CallStatsGroup = "model"
	GroupByServedProvider CallStatsGroup = "served_provider"
	GroupByTier           CallStatsGroup = "tier"
	GroupByTask           CallStatsGroup = "task"
)

// callStatsColumns maps each grouping to its column, a compile-time literal:
// the grouping arrives on a query string and is never formatted in itself.
var callStatsColumns = map[CallStatsGroup]string{
	GroupByProvider: "ac.provider", GroupByModel: "ac.model_id", GroupByServedProvider: "ac.served_provider",
	GroupByTier: "ac.tier", GroupByTask: "ac.task",
}

// CallStatsWindows are the windows the screens offer.
var CallStatsWindows = map[string]time.Duration{"24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour}

// CallStatsFilter narrows the attempts counted; an empty field filters nothing.
type CallStatsFilter struct {
	Provider, Model, Tier string
	Task                  Task
}

// CallStatsQuery is one figures read.
type CallStatsQuery struct {
	Window  time.Duration
	GroupBy CallStatsGroup
	Filter  CallStatsFilter
}

// CallStatsRow is one group's figures over the window's attempts.
type CallStatsRow struct {
	Key          string
	Calls        int64
	Failed       int64
	Timeouts     int64
	P50Ms        int64
	P95Ms        int64
	TokensIn     int64
	TokensOut    int64
	CostMicroUSD int64
	Unpriced     int64
}

// failedAttemptSQL is whether the ai_call row aliased alias failed: it carries
// a sentinel that is not an answer. The sentinel list is bound at answeredArg,
// from answeredSentinels, so the SQL never spells the list itself.
func failedAttemptSQL(alias string, answeredArg int) string {
	return fmt.Sprintf(`(%[1]s.error_sentinel IS NOT NULL AND %[1]s.error_sentinel <> '' AND NOT %[1]s.error_sentinel = ANY($%[2]d))`, alias, answeredArg)
}

// CallStats counts the window's attempts, cache hits excluded: one never
// reached a provider. A window with no attempts is an empty slice.
func (s *CallReadStore) CallStats(ctx context.Context, q CallStatsQuery) ([]CallStatsRow, error) {
	if err := auth.Require(ctx, "ai_diagnostics", principal.ActionRead); err != nil {
		return nil, err
	}
	column, ok := callStatsColumns[q.GroupBy]
	if !ok {
		return nil, invalidAt("group", "must be one of provider, model, served_provider, tier, task")
	}
	from, to := rateFallbacks()
	args := []any{s.now().Add(-q.Window), answeredSentinels}
	failed := failedAttemptSQL("ac", len(args))
	args = append(args, from, to)
	rate := rateMatch("ac.provider", "ac.model_id", "ac.occurred_at::date", len(args)-1, len(args))
	// A row with no tier is a call refused before any rung ran, not an attempt.
	where := "ac.occurred_at >= $1 AND NOT ac.cache_hit AND ac.tier <> ''"
	var whereSb101 strings.Builder
	for _, f := range []struct{ column, value string }{
		{"ac.provider", q.Filter.Provider}, {"ac.model_id", q.Filter.Model}, {"ac.tier", q.Filter.Tier}, {"ac.task", string(q.Filter.Task)},
	} {
		if f.value != "" {
			args = append(args, f.value)
			fmt.Fprintf(&whereSb101, " AND %s = $%d", f.column, len(args))
		}
	}
	where += whereSb101.String()
	sql := storekit.SQLf(`
		SELECT %[1]s AS key,
		       count(*),
		       count(*) FILTER (WHERE %[2]s),
		       count(*) FILTER (WHERE ac.error_sentinel = '`+sentinelTimeout+`'),
		       coalesce(percentile_disc(0.5) WITHIN GROUP (ORDER BY ac.latency_ms), 0),
		       coalesce(percentile_disc(0.95) WITHIN GROUP (ORDER BY ac.latency_ms), 0),
		       coalesce(sum(ac.tokens_in), 0), coalesce(sum(ac.tokens_out), 0),
		       coalesce(sum(%[3]s), 0)::bigint,
		       count(*) FILTER (WHERE %[4]s)
		  FROM ai_call ac
		  LEFT JOIN LATERAL (%[5]s) r ON true
		 WHERE %[6]s
		 GROUP BY 1
		 ORDER BY 2 DESC, 1`,
		column, failed, callCostMicroUSD("ac"), callUnpriced("ac"),
		rate, where)
	rows := []CallStatsRow{}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		found, err := tx.Query(ctx, sql, args...)
		if err != nil {
			return err
		}
		defer found.Close()
		for found.Next() {
			var row CallStatsRow
			if err := found.Scan(&row.Key, &row.Calls, &row.Failed, &row.Timeouts, &row.P50Ms, &row.P95Ms,
				&row.TokensIn, &row.TokensOut, &row.CostMicroUSD, &row.Unpriced); err != nil {
				return err
			}
			rows = append(rows, row)
		}
		return found.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("ai: reading call figures: %w", err)
	}
	return rows, nil
}

// FlowStep is one step of a task's route: the decision model, or one tier.
type FlowStep struct {
	Decision bool
	Tier     Tier
	Provider string
	Model    string
	Attempts int64
	// Answered counts the logical calls this step answered, which is the
	// terminal attempt landing here without a failure.
	Answered int64
	P50Ms    int64
	// GaveUp counts why the walk moved past this step: the attempt's own
	// sentinel when it failed, else the reason the next attempt was made.
	GaveUp map[string]int64
}

// TaskFlow is what one task's calls did over a window.
type TaskFlow struct {
	Total      int64
	Unanswered int64
	Steps      []FlowStep
}

// TaskFlow reads task's route over the window: the decision step first, then
// each tier in ladder order. Unanswered counts the logical calls whose last
// attempt failed.
func (s *CallReadStore) TaskFlow(ctx context.Context, task Task, window time.Duration) (TaskFlow, error) {
	if err := auth.Require(ctx, "ai_diagnostics", principal.ActionRead); err != nil {
		return TaskFlow{}, err
	}
	flow := TaskFlow{Steps: []FlowStep{}}
	args := []any{string(task), s.now().Add(-window), answeredSentinels}
	failed := failedAttemptSQL("ac", len(args))
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			SELECT count(DISTINCT ac.logical_call_id),
			       count(DISTINCT ac.logical_call_id) FILTER (WHERE ac.is_terminal AND `+failed+`)
			  FROM ai_call ac
			 WHERE ac.task = $1 AND ac.occurred_at >= $2 AND NOT ac.cache_hit`,
			args...).Scan(&flow.Total, &flow.Unanswered); err != nil {
			return err
		}
		steps, err := s.flowSteps(ctx, tx, task, args, failed)
		flow.Steps = steps
		return err
	})
	if err != nil {
		return TaskFlow{}, fmt.Errorf("ai: reading a task's call flow: %w", err)
	}
	return flow, nil
}

// flowSteps reads with TaskFlow's arguments: $1 task, $2 since, and the answered
// sentinels that failed binds.
func (s *CallReadStore) flowSteps(ctx context.Context, tx pgx.Tx, task Task, args []any, failed string) ([]FlowStep, error) {
	found, err := tx.Query(ctx, `
		WITH attempts AS (
		  SELECT ac.kind = 'decision' AS decision, ac.tier, ac.provider, ac.model_id, ac.latency_ms, ac.is_terminal,
		         `+failed+` AS failed,
		         coalesce(ac.error_sentinel, '') AS sentinel,
		         lead(ac.attempt_reason) OVER (PARTITION BY ac.logical_call_id ORDER BY ac.attempt) AS next_reason
		    FROM ai_call ac
		   WHERE ac.task = $1 AND ac.occurred_at >= $2 AND NOT ac.cache_hit
		)
		SELECT decision, tier,
		       mode() WITHIN GROUP (ORDER BY provider), mode() WITHIN GROUP (ORDER BY model_id),
		       count(*), count(*) FILTER (WHERE is_terminal AND NOT failed),
		       coalesce(percentile_disc(0.5) WITHIN GROUP (ORDER BY latency_ms), 0),
		       coalesce(array_agg(CASE WHEN sentinel <> '' THEN sentinel ELSE next_reason END)
		                FILTER (WHERE NOT (is_terminal AND NOT failed)), '{}')
		  FROM attempts
		 GROUP BY decision, tier`, args...)
	if err != nil {
		return nil, err
	}
	defer found.Close()
	var steps []FlowStep
	for found.Next() {
		step := FlowStep{GaveUp: map[string]int64{}}
		var reasons []*string
		if err := found.Scan(&step.Decision, &step.Tier, &step.Provider, &step.Model,
			&step.Attempts, &step.Answered, &step.P50Ms, &reasons); err != nil {
			return nil, err
		}
		for _, reason := range reasons {
			if reason != nil && *reason != "" {
				step.GaveUp[*reason]++
			}
		}
		steps = append(steps, step)
	}
	if err := found.Err(); err != nil {
		return nil, err
	}
	slices.SortStableFunc(steps, func(a, b FlowStep) int { return routeOrder(task, a) - routeOrder(task, b) })
	return steps, nil
}

// routeOrder places a step in the order the router tries them: the decision
// model, then the ladder's rungs, then any tier the ladder no longer names.
func routeOrder(task Task, step FlowStep) int {
	if step.Decision {
		return -1
	}
	if i := slices.Index(taskLadders[task], step.Tier); i >= 0 {
		return i
	}
	return len(taskLadders[task])
}
