// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// Reading and writing a review row.
//
// Split from the types and the engine's wiring beside it: those say what a
// review IS and which seams answer for it, and this is the SQL that puts one in
// the table and takes it back out.

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// LatestReview serves the acting rep's most recent weekly review, or the one
// for a named week.
//
// It never assembles. A retrospective is written when the week closed, and a
// read that could re-derive it would answer differently depending on when it
// was asked — which is the one thing a record of a past week must not do.
//
// Held by: TestASecondAssemblyInOneWeekReadsTheFirst
// (backend/internal/compose/weekly/weekly_integration_test.go)
func (e *Engine) LatestReview(ctx context.Context, weekStart *time.Time) (Review, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return Review{}, err
	}
	userID, err := reviewUser(ctx)
	if err != nil {
		return Review{}, err
	}

	var review Review
	err = database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, reviewSelect+`
			 WHERE user_id = $1 AND ($2::date IS NULL OR local_week_start = $2)
			 ORDER BY local_week_start DESC
			 LIMIT 1`, userID, weekStart)
		var err error
		if review, err = scanReview(ctx, tx, row); err != nil {
			return err
		}
		review.Prior, err = readPriorWeek(ctx, tx, review.PriorReviewID, userID)
		return err
	})
	if err != nil {
		return Review{}, err
	}
	return review, nil
}

// reviewSelect is the review's columns, in the order scanReview reads them.
//
// ONE spelling for both readers — the screen's and the mail's. Two copies of
// this list is how the same week comes to render two different ways, which is
// the one thing a record of a past week must never do.
const reviewSelect = `
	SELECT id, user_id, local_week_start, generated_at, as_of,
	       tasks_due, tasks_done, tasks_carried_over,
	       deals_moved, deals_won, deals_lost,
	       proposals_accepted, proposals_rejected,
	       brief_items_acted, brief_items_dismissed,
	       commitments_due, commitments_kept,
	       leads_routed, leads_answered_in_target, leads_breached,
	       meetings_held, meetings_with_next_step,
	       pipeline_created_minor, pipeline_won_minor, pipeline_lost_minor,
	       base_currency, prior_review_id,
	       coalesce(narrative, ''), narrated_at
	  FROM weekly_review`

// scanReview reads one review row and its frozen deal lines.
func scanReview(ctx context.Context, tx pgx.Tx, row pgx.Row) (Review, error) {
	var review Review
	c := &review.Counts
	var created, won, lost *int64
	var currency *string
	switch err := row.Scan(&review.ID, &review.UserID, &review.LocalWeekStart,
		&review.GeneratedAt, &review.AsOf,
		&c.TasksDue, &c.TasksDone, &c.TasksCarriedOver,
		&c.DealsMoved, &c.DealsWon, &c.DealsLost,
		&c.ProposalsAccepted, &c.ProposalsRejected,
		&c.BriefItemsActed, &c.BriefItemsDismissed,
		&c.CommitmentsDue, &c.CommitmentsKept,
		&c.LeadsRouted, &c.LeadsAnsweredInTarget, &c.LeadsBreached,
		&c.MeetingsHeld, &c.MeetingsWithNextStep,
		&created, &won, &lost, &currency, &review.PriorReviewID,
		&review.Narrative, &review.NarratedAt); {
	case errors.Is(err, pgx.ErrNoRows):
		return Review{}, apperrors.ErrNotFound
	case err != nil:
		return Review{}, err
	}
	// Known reads off the CURRENCY, which the table's own CHECK ties to the
	// three figures: either all four are present or none is. Reading it off a
	// sum would call a genuine zero — a week that created no pipeline — an
	// unconvertible one.
	if currency != nil {
		review.Money = Money{
			CreatedMinor: deref(created), WonMinor: deref(won), LostMinor: deref(lost),
			Currency: *currency, Known: true,
		}
	}
	lines, err := readDealLines(ctx, tx, review.ID)
	if err != nil {
		return Review{}, err
	}
	review.Deals = lines
	outlook, err := readOutlook(ctx, tx, review.ID)
	if err != nil {
		return Review{}, err
	}
	review.Outlook = outlook
	return review, nil
}

// readReviewTx reads one review by id, scoped to the rep whose week it was.
func readReviewTx(ctx context.Context, tx pgx.Tx, reviewID, userID ids.UUID) (Review, error) {
	return scanReview(ctx, tx,
		tx.QueryRow(ctx, reviewSelect+` WHERE id = $1 AND user_id = $2`, reviewID, userID))
}

// ListWeeks serves the weeks this rep has a review for, newest first — the
// archive's index. Counts only, no deal lines: the index is a list of doors.
func (e *Engine) ListWeeks(ctx context.Context, limit int) ([]time.Time, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	userID, err := reviewUser(ctx)
	if err != nil {
		return nil, err
	}
	var weeks []time.Time
	err = database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT local_week_start FROM weekly_review
			 WHERE user_id = $1 ORDER BY local_week_start DESC LIMIT $2`, userID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var week time.Time
			if err := rows.Scan(&week); err != nil {
				return err
			}
			weeks = append(weeks, week)
		}
		return rows.Err()
	})
	return weeks, err
}

// readDealLines reads one review's frozen deal lines.
//
// It joins NOTHING. Every word it returns was written when the review was, so
// a deal renamed, archived or deleted since leaves the line exactly as the
// week recorded it.
func readDealLines(ctx context.Context, tx pgx.Tx, reviewID ids.UUID) ([]DealLine, error) {
	rows, err := tx.Query(ctx, `
		SELECT deal_id, deal_label, outcome, coalesce(to_stage_label, ''),
		       amount_minor_at_close, coalesce(currency_at_close, ''), occurred_at
		  FROM weekly_review_deal
		 WHERE weekly_review_id = $1
		 -- The SAME order the assembly wrote them in: won and lost first,
		 -- because that is what a week is remembered by. Ordering the read
		 -- differently would let a later stage move rise above an older win,
		 -- so the review would read differently from the one that was written.
		 ORDER BY (outcome = 'moved'), occurred_at DESC, deal_id`, reviewID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lines []DealLine
	for rows.Next() {
		var line DealLine
		if err := rows.Scan(&line.DealID, &line.Label, &line.Outcome,
			&line.ToStageLabel, &line.AmountMinor, &line.Currency, &line.OccurredAt); err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

// insertReview writes the review unless this rep already has one for the same
// local week, reporting which of the two happened.
//
// The unique constraint is the arbiter rather than the read that precedes it:
// the dispatcher ticks more than once inside a week on purpose, so a worker
// that was down still backfills, and two writers racing produce one review and
// no error.
func insertReview(ctx context.Context, tx pgx.Tx, review Review) (ids.UUID, bool, error) {
	c := review.Counts
	// Column and value paired at the point of writing, and the placeholders
	// derived from the argument slice. Twenty-odd hand-numbered $N in one
	// statement is a column-and-argument miscount waiting to happen, and
	// nothing checks that the three lists agree.
	cols, args := insertColumns{}, []any(nil)
	add := func(name string, value any) {
		cols = append(cols, name)
		args = append(args, value)
	}
	add("user_id", review.UserID)
	add("local_week_start", review.LocalWeekStart)
	add("as_of", review.AsOf)
	add("tasks_due", c.TasksDue)
	add("tasks_done", c.TasksDone)
	add("tasks_carried_over", c.TasksCarriedOver)
	add("deals_moved", c.DealsMoved)
	add("deals_won", c.DealsWon)
	add("deals_lost", c.DealsLost)
	add("proposals_accepted", c.ProposalsAccepted)
	add("proposals_rejected", c.ProposalsRejected)
	add("brief_items_acted", c.BriefItemsActed)
	add("brief_items_dismissed", c.BriefItemsDismissed)
	add("leads_routed", c.LeadsRouted)
	add("leads_answered_in_target", c.LeadsAnsweredInTarget)
	add("leads_breached", c.LeadsBreached)
	add("meetings_held", c.MeetingsHeld)
	add("meetings_with_next_step", c.MeetingsWithNextStep)
	add("commitments_due", c.CommitmentsDue)
	add("commitments_kept", c.CommitmentsKept)
	add("prior_review_id", review.PriorReviewID)
	// The four money columns are written together or not at all — the table's
	// own CHECK says a figure names its currency and a currency names figures.
	// An unconvertible week writes four nulls, which is the honest absence.
	if review.Money.Known {
		add("pipeline_created_minor", review.Money.CreatedMinor)
		add("pipeline_won_minor", review.Money.WonMinor)
		add("pipeline_lost_minor", review.Money.LostMinor)
		add("base_currency", review.Money.Currency)
	}

	var id ids.UUID
	err := tx.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO weekly_review (%s) VALUES (%s)
		ON CONFLICT ON CONSTRAINT uq_weekly_review_user_week DO NOTHING
		RETURNING id`, cols.names(), cols.placeholders()), args...).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		// The week already had a review. The loser of that race is not an
		// error: the constraint did its job.
		return ids.Nil, false, nil
	}
	if err != nil {
		return ids.Nil, false, fmt.Errorf("weekly: writing the review: %w", err)
	}
	if _, err := storekit.Audit(ctx, tx, "create", "weekly_review", id, nil,
		map[string]any{"local_week_start": review.LocalWeekStart, "as_of": review.AsOf}); err != nil {
		return ids.Nil, false, err
	}
	return id, true, nil
}

type insertColumns []string

func (c insertColumns) names() string { return strings.Join(c, ", ") }

func (c insertColumns) placeholders() string {
	holders := make([]string, len(c))
	for i := range c {
		holders[i] = "$" + strconv.Itoa(i+1)
	}
	return strings.Join(holders, ", ")
}
