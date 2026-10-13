// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// Writing the team's frozen week: the parent row, then its children in the
// same transaction.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// teamReviewChildTables are the tables hanging off team_weekly_review. The
// zero-member replacement path empties them, so a corrected snapshot keeps no
// child of the one it replaced. Rep children cascade.
//
// Held by: TestTheReplacementEmptiesEveryChildOfTheTeamReview
// (backend/internal/compose/weekly/teamreviewwrite_test.go)
var teamReviewChildTables = []string{
	"team_weekly_review_outlook",
	"team_weekly_review_coverage",
	"team_weekly_review_member",
	"team_weekly_review_rep",
}

// insertTeamReview writes the snapshot unless this team already has one for the
// week, reporting which of the two happened.
func insertTeamReview(ctx context.Context, tx pgx.Tx, review TeamReview) (ids.UUID, bool, error) {
	cols, args := teamReviewColumns(review)
	updates := []string{"generated_at = now()"}
	for _, col := range cols {
		updates = append(updates, col+" = EXCLUDED."+col)
	}

	var id ids.UUID
	var inserted bool
	err := tx.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO team_weekly_review (%s) VALUES (%s)
		ON CONFLICT ON CONSTRAINT uq_team_weekly_review_team_week DO UPDATE SET %s
		WHERE team_weekly_review.reps_counted = 0 AND EXCLUDED.reps_counted > 0
		RETURNING id, (xmax = 0)`, cols.names(), cols.placeholders(), strings.Join(updates, ", ")), args...).Scan(&id, &inserted)
	if errors.Is(err, pgx.ErrNoRows) {
		// The team's week already had a snapshot. The loser of that race is not
		// an error: the constraint did its job.
		return ids.Nil, false, nil
	}
	if err != nil {
		return ids.Nil, false, fmt.Errorf("weekly: writing the team's week: %w", err)
	}
	after := map[string]any{
		"local_week_start": review.LocalWeekStart, "reps_counted": review.Counts.RepsCounted,
		"summary": review.Summary,
	}
	if inserted {
		if _, err := storekit.Audit(ctx, tx, "create", "team_weekly_review", id, nil, after); err != nil {
			return ids.Nil, false, err
		}
		return id, true, nil
	}
	for _, table := range teamReviewChildTables {
		// Identifiers from the constant list above, never from a request.
		if _, err := tx.Exec(ctx, `DELETE FROM `+table+` WHERE team_weekly_review_id = $1`, id); err != nil {
			return ids.Nil, false, fmt.Errorf("weekly: replacing an unmeasured snapshot's %s: %w", table, err)
		}
	}
	if _, err := storekit.Audit(ctx, tx, "update", "team_weekly_review", id, map[string]any{"reps_counted": 0}, after); err != nil {
		return ids.Nil, false, err
	}
	return id, true, nil
}

// insertTeamChildren writes everything the snapshot holds besides its row.
func insertTeamChildren(ctx context.Context, tx pgx.Tx, review TeamReview) error {
	if err := insertTeamMembers(ctx, tx, review.ID, review.Members); err != nil {
		return err
	}
	if err := insertTeamReps(ctx, tx, review.ID, review.Reps); err != nil {
		return err
	}
	for _, outlook := range review.Outlook {
		if err := insertOutlookInto(ctx, tx, teamOutlook, review.ID, outlook); err != nil {
			return err
		}
	}
	for _, coverage := range review.Coverage {
		if err := insertTeamCoverage(ctx, tx, review.ID, coverage); err != nil {
			return err
		}
	}
	return nil
}

// insertTeamMembers freezes the whole roster, measured or not.
func insertTeamMembers(ctx context.Context, tx pgx.Tx, reviewID ids.UUID, members []FrozenMember) error {
	for _, m := range members {
		if _, err := tx.Exec(ctx, `
			INSERT INTO team_weekly_review_member (team_weekly_review_id, user_id, display_name, measured)
			VALUES ($1, $2, $3, $4)`, reviewID, m.UserID, m.DisplayName, m.Measured); err != nil {
			return fmt.Errorf("weekly: writing the team's roster: %w", err)
		}
	}
	return nil
}

// insertTeamReps writes the measured members, each with its patterns and its
// funnel deviations.
func insertTeamReps(ctx context.Context, tx pgx.Tx, reviewID ids.UUID, reps []TeamRep) error {
	for _, rep := range reps {
		var repID ids.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO team_weekly_review_rep
			    (team_weekly_review_id, user_id, display_name, deals_won, leads_breached,
			     meetings_held, commitments_due, commitments_kept, help_requested,
			     focus_kind, focus_label)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			RETURNING id`,
			reviewID, rep.UserID, rep.DisplayName, rep.DealsWon, rep.LeadsBreached,
			rep.MeetingsHeld, rep.CommitmentsDue, rep.CommitmentsKept, rep.HelpRequested,
			rep.FocusKind, rep.FocusLabel).Scan(&repID); err != nil {
			return fmt.Errorf("weekly: writing a member's row: %w", err)
		}
		for position, p := range rep.Patterns {
			if _, err := tx.Exec(ctx, `
				INSERT INTO team_weekly_review_rep_pattern (team_weekly_review_rep_id, "position", kind, label)
				VALUES ($1, $2, $3, $4)`, repID, position, p.Kind, p.Label); err != nil {
				return fmt.Errorf("weekly: writing a member's pattern: %w", err)
			}
		}
		if err := insertRepFunnel(ctx, tx, repID, rep.Funnel); err != nil {
			return err
		}
	}
	return nil
}

// insertRepFunnel writes a rep's ranked funnel deviations, position from 1.
func insertRepFunnel(ctx context.Context, tx pgx.Tx, repID ids.UUID, funnel []FunnelDeviation) error {
	for i, f := range funnel {
		cols, args := insertColumns{}, []any(nil)
		add := func(name string, value any) {
			cols = append(cols, name)
			args = append(args, value)
		}
		add("team_weekly_review_rep_id", repID)
		add(`"position"`, i+1)
		add("pipeline_id", f.PipelineID)
		add("pipeline_name", f.PipelineName)
		add("stage_id", f.StageID)
		add("stage_name", f.StageName)
		add("next_stage_name", nullableText(f.NextStageName))
		add("rep_entered", f.RepEntered)
		add("rep_converted", f.RepConverted)
		add("team_entered", f.TeamEntered)
		add("team_converted", f.TeamConverted)
		add("deviation_bp", f.DeviationBp)
		if _, err := tx.Exec(ctx, fmt.Sprintf(`INSERT INTO team_weekly_review_rep_funnel (%s) VALUES (%s)`,
			cols.names(), cols.placeholders()), args...); err != nil {
			return fmt.Errorf("weekly: writing a member's funnel: %w", err)
		}
	}
	return nil
}

// insertTeamCoverage freezes one horizon's coverage.
func insertTeamCoverage(ctx context.Context, tx pgx.Tx, reviewID ids.UUID, c Coverage) error {
	cols, args := insertColumns{}, []any(nil)
	add := func(name string, value any) {
		cols = append(cols, name)
		args = append(args, value)
	}
	add("team_weekly_review_id", reviewID)
	add("period_kind", c.PeriodKind)
	add("basis", nullableText(c.Basis))
	add("absent", nullableText(c.Absent))
	add("base_currency", c.BaseCurrency)
	add("replan_recommended", c.ReplanRecommended)
	if c.Basis != "" {
		add("reference_landing_minor", c.ReferenceLandingMinor)
		add("needed_open_minor", c.NeededOpenMinor)
		add("current_open_minor", c.CurrentOpenMinor)
		add("remaining_to_support_minor", c.RemainingToSupportMinor)
		add("coverage_bp", c.CoverageBp)
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`INSERT INTO team_weekly_review_coverage (%s) VALUES (%s)`,
		cols.names(), cols.placeholders()), args...); err != nil {
		return fmt.Errorf("weekly: writing the team's coverage: %w", err)
	}
	return nil
}

// teamReviewColumns are the parent row's columns and values, in one order.
func teamReviewColumns(review TeamReview) (insertColumns, []any) {
	c := review.Counts
	cols, args := insertColumns{}, []any(nil)
	add := func(name string, value any) {
		cols = append(cols, name)
		args = append(args, value)
	}
	add("numeric_summary", review.NumericSummary)
	add("team_id", review.TeamID)
	add("team_name", review.TeamName)
	add("local_week_start", review.LocalWeekStart)
	add("as_of", review.AsOf)
	add("reps_counted", c.RepsCounted)
	add("reps_unread", review.RepsUnread)
	add("deals_won", c.DealsWon)
	add("deals_lost", c.DealsLost)
	add("deals_moved", c.DealsMoved)
	add("leads_routed", c.LeadsRouted)
	add("leads_answered_in_target", c.LeadsAnsweredInTarget)
	add("leads_breached", c.LeadsBreached)
	add("meetings_held", c.MeetingsHeld)
	add("meetings_with_next_step", c.MeetingsWithNextStep)
	add("commitments_due", c.CommitmentsDue)
	add("commitments_kept", c.CommitmentsKept)
	var created, won, lost *int64
	var currency *string
	if review.Money.Known {
		created, won, lost = &review.Money.CreatedMinor, &review.Money.WonMinor, &review.Money.LostMinor
		currency = &review.Money.Currency
	}
	add("pipeline_created_minor", created)
	add("pipeline_won_minor", won)
	add("pipeline_lost_minor", lost)
	add("base_currency", currency)
	add("proposals_accepted", review.Quality.ProposalsAccepted)
	add("proposals_rejected", review.Quality.ProposalsRejected)
	add("auto_change_reversals", review.Quality.AutoChangeReversals)
	add("summary", nullableText(review.Summary))
	return cols, args
}
