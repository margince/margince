// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// Reading the team snapshot's children.
//
// They join nothing. Every word was written when the snapshot was, so a rep
// renamed or a stage deleted since leaves the week as it recorded it.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// readTeamReps reads the measured members, ordered as the page draws it, with
// their patterns and funnel deviations.
func readTeamReps(ctx context.Context, tx pgx.Tx, reviewID ids.UUID) ([]TeamRep, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, user_id, display_name, deals_won, leads_breached, meetings_held,
		       commitments_due, commitments_kept, help_requested, focus_kind, focus_label
		  FROM team_weekly_review_rep
		 WHERE team_weekly_review_id = $1
		 ORDER BY display_name, user_id`, reviewID)
	if err != nil {
		return nil, fmt.Errorf("weekly: reading the team's members: %w", err)
	}
	var repIDs []ids.UUID
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (TeamRep, error) {
		var rep TeamRep
		var id ids.UUID
		err := row.Scan(&id, &rep.UserID, &rep.DisplayName, &rep.DealsWon,
			&rep.LeadsBreached, &rep.MeetingsHeld, &rep.CommitmentsDue,
			&rep.CommitmentsKept, &rep.HelpRequested, &rep.FocusKind, &rep.FocusLabel)
		repIDs = append(repIDs, id)
		return rep, err
	})
	if err != nil {
		return nil, fmt.Errorf("weekly: reading the team's members: %w", err)
	}
	patterns, err := readRepPatterns(ctx, tx, repIDs)
	if err != nil {
		return nil, err
	}
	funnels, err := readRepFunnels(ctx, tx, repIDs)
	if err != nil {
		return nil, err
	}
	for i, id := range repIDs {
		out[i].Patterns = frozenPatterns(out[i], patterns[id])
		out[i].Funnel = funnels[id]
	}
	return out, nil
}

// frozenPatterns are a rep's stored patterns. A rep frozen before patterns were
// kept reads its focus as the one pattern. So every rep carries one or more, and
// the first is the focus.
func frozenPatterns(rep TeamRep, stored []Pattern) []Pattern {
	if len(stored) == 0 {
		return []Pattern{{Kind: rep.FocusKind, Label: rep.FocusLabel}}
	}
	return stored
}

// readRepPatterns reads every rep's patterns in one query, keyed by rep row.
func readRepPatterns(ctx context.Context, tx pgx.Tx, repIDs []ids.UUID) (map[ids.UUID][]Pattern, error) {
	out := map[ids.UUID][]Pattern{}
	if len(repIDs) == 0 {
		return out, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT team_weekly_review_rep_id, kind, label FROM team_weekly_review_rep_pattern
		 WHERE team_weekly_review_rep_id = ANY($1)
		 ORDER BY team_weekly_review_rep_id, "position"`, repIDs)
	if err != nil {
		return nil, fmt.Errorf("weekly: reading the members' patterns: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var rep ids.UUID
		var p Pattern
		if err := rows.Scan(&rep, &p.Kind, &p.Label); err != nil {
			return nil, err
		}
		out[rep] = append(out[rep], p)
	}
	return out, rows.Err()
}

// readRepFunnels reads every rep's funnel deviations in one query.
func readRepFunnels(ctx context.Context, tx pgx.Tx, repIDs []ids.UUID) (map[ids.UUID][]FunnelDeviation, error) {
	out := map[ids.UUID][]FunnelDeviation{}
	if len(repIDs) == 0 {
		return out, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT team_weekly_review_rep_id, pipeline_id, pipeline_name, stage_id, stage_name,
		       coalesce(next_stage_name, ''), rep_entered, rep_converted, team_entered,
		       team_converted, deviation_bp
		  FROM team_weekly_review_rep_funnel
		 WHERE team_weekly_review_rep_id = ANY($1)
		 ORDER BY team_weekly_review_rep_id, "position"`, repIDs)
	if err != nil {
		return nil, fmt.Errorf("weekly: reading the members' funnels: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var rep ids.UUID
		var f FunnelDeviation
		if err := rows.Scan(&rep, &f.PipelineID, &f.PipelineName, &f.StageID, &f.StageName,
			&f.NextStageName, &f.RepEntered, &f.RepConverted, &f.TeamEntered,
			&f.TeamConverted, &f.DeviationBp); err != nil {
			return nil, err
		}
		out[rep] = append(out[rep], f)
	}
	return out, rows.Err()
}

// readTeamMembers reads the frozen roster, measured or not.
func readTeamMembers(ctx context.Context, tx pgx.Tx, reviewID ids.UUID) ([]FrozenMember, error) {
	rows, err := tx.Query(ctx, `
		SELECT user_id, display_name, measured FROM team_weekly_review_member
		 WHERE team_weekly_review_id = $1 ORDER BY display_name, user_id`, reviewID)
	if err != nil {
		return nil, fmt.Errorf("weekly: reading the team's roster: %w", err)
	}
	members, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (FrozenMember, error) {
		var m FrozenMember
		err := row.Scan(&m.UserID, &m.DisplayName, &m.Measured)
		return m, err
	})
	if err != nil {
		return nil, fmt.Errorf("weekly: reading the team's roster: %w", err)
	}
	return members, nil
}

// readTeamCoverage reads the frozen coverage, month before quarter.
func readTeamCoverage(ctx context.Context, tx pgx.Tx, reviewID ids.UUID) ([]Coverage, error) {
	rows, err := tx.Query(ctx, `
		SELECT period_kind, coalesce(basis, ''), coalesce(absent, ''),
		       coalesce(reference_landing_minor, 0), coalesce(needed_open_minor, 0),
		       coalesce(current_open_minor, 0), coalesce(remaining_to_support_minor, 0),
		       coalesce(coverage_bp, 0), base_currency, replan_recommended
		  FROM team_weekly_review_coverage
		 WHERE team_weekly_review_id = $1
		 ORDER BY CASE period_kind WHEN 'month' THEN 0 ELSE 1 END`, reviewID)
	if err != nil {
		return nil, fmt.Errorf("weekly: reading the team's coverage: %w", err)
	}
	coverage, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Coverage, error) {
		var c Coverage
		err := row.Scan(&c.PeriodKind, &c.Basis, &c.Absent, &c.ReferenceLandingMinor,
			&c.NeededOpenMinor, &c.CurrentOpenMinor, &c.RemainingToSupportMinor,
			&c.CoverageBp, &c.BaseCurrency, &c.ReplanRecommended)
		return c, err
	})
	if err != nil {
		return nil, fmt.Errorf("weekly: reading the team's coverage: %w", err)
	}
	return coverage, nil
}
