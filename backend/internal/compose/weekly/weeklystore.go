// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package weekly assembles and serves one rep's weekly retrospective.
//
// ITS OWN AGGREGATE, and the reasons are two different ones.
//
// It is not a brief_run because briefLastView orders that table by
// generated_at to decide the next morning's overnight window: a weekly row
// there would become "the latest brief" and silently reset what Saturday's
// brief counts as changed overnight.
//
// It is not brief_item because brief_item.deal_id cascades from deal. That is
// right for a queue, which is about deals that exist. It is wrong for a
// retrospective, which is a record of what a week WAS — a past week that
// quietly loses a line because somebody cleaned up a deal is a record nobody
// can trust. So the deal lines here are FROZEN: the id is stored without a
// foreign key, beside the label the deal carried that week.
package weekly

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/weekly/narrative"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Review is one rep's week, as it was measured.
type Review struct {
	ID             ids.UUID
	UserID         ids.UUID
	LocalWeekStart time.Time
	GeneratedAt    time.Time
	AsOf           time.Time
	Counts         Counts
	Deals          []DealLine
	// Narrative is the sentence a model wrote about the week, empty when none
	// did. NarratedAt is what tells "no pass ran" from "a pass ran and found
	// the week unremarkable" — the two read identically as silence otherwise.
	Narrative  string
	NarratedAt *time.Time
}

// Counts are the week's tallies. Every one is as-of the review's AsOf, which
// is why they are stored rather than recomputed: a retrospective that changes
// when you reopen it is not a retrospective.
type Counts struct {
	TasksDue            int
	TasksDone           int
	TasksCarriedOver    int
	DealsMoved          int
	DealsWon            int
	DealsLost           int
	ProposalsAccepted   int
	ProposalsRejected   int
	BriefItemsActed     int
	BriefItemsDismissed int
}

// DealLine is one deal the week is about, frozen at the moment it was written.
type DealLine struct {
	DealID ids.UUID
	// Label is what the deal was called that week. A rename does not rewrite
	// history and a deletion does not erase it.
	Label   string
	Outcome string
	// ToStageLabel is where it went, as words: a renamed or deleted stage must
	// not make an old review unreadable.
	ToStageLabel string
	AmountMinor  *int64
	Currency     string
	OccurredAt   time.Time
}

// The three outcomes a deal line records.
const (
	OutcomeMoved = "moved"
	OutcomeWon   = "won"
	OutcomeLost  = "lost"
)

// Engine assembles and reads weekly reviews.
type Engine struct{ pool *pgxpool.Pool }

// NewEngine binds the engine to the installation pool.
func NewEngine(pool *pgxpool.Pool) *Engine { return &Engine{pool: pool} }

// reviewUser is the acting rep. A review is a personal record — whose week it
// was — so there is no argument by which a caller asks for somebody else's.
func reviewUser(ctx context.Context) (ids.UUID, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return ids.Nil, apperrors.ErrPermissionDenied
	}
	return actor.UserID, nil
}

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
		row := tx.QueryRow(ctx, `
			SELECT id, user_id, local_week_start, generated_at, as_of,
			       tasks_due, tasks_done, tasks_carried_over,
			       deals_moved, deals_won, deals_lost,
			       proposals_accepted, proposals_rejected,
			       brief_items_acted, brief_items_dismissed,
			       coalesce(narrative, ''), narrated_at
			  FROM weekly_review
			 WHERE user_id = $1 AND ($2::date IS NULL OR local_week_start = $2)
			 ORDER BY local_week_start DESC
			 LIMIT 1`, userID, weekStart)
		c := &review.Counts
		switch err := row.Scan(&review.ID, &review.UserID, &review.LocalWeekStart,
			&review.GeneratedAt, &review.AsOf,
			&c.TasksDue, &c.TasksDone, &c.TasksCarriedOver,
			&c.DealsMoved, &c.DealsWon, &c.DealsLost,
			&c.ProposalsAccepted, &c.ProposalsRejected,
			&c.BriefItemsActed, &c.BriefItemsDismissed,
			&review.Narrative, &review.NarratedAt); {
		case errors.Is(err, pgx.ErrNoRows):
			return apperrors.ErrNotFound
		case err != nil:
			return err
		}
		review.Deals, err = readDealLines(ctx, tx, review.ID)
		return err
	})
	if err != nil {
		return Review{}, err
	}
	return review, nil
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
	var id ids.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO weekly_review (user_id, local_week_start, as_of,
		    tasks_due, tasks_done, tasks_carried_over,
		    deals_moved, deals_won, deals_lost,
		    proposals_accepted, proposals_rejected,
		    brief_items_acted, brief_items_dismissed)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT ON CONSTRAINT uq_weekly_review_user_week DO NOTHING
		RETURNING id`,
		review.UserID, review.LocalWeekStart, review.AsOf,
		c.TasksDue, c.TasksDone, c.TasksCarriedOver,
		c.DealsMoved, c.DealsWon, c.DealsLost,
		c.ProposalsAccepted, c.ProposalsRejected,
		c.BriefItemsActed, c.BriefItemsDismissed).Scan(&id)
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

// Narrate writes the week's sentence onto an existing review.
//
// A SECOND WRITE onto a row the deterministic pass already committed, never
// part of assembling it. The counts and the deal lines are the review; this is
// what a colleague would say about them, and it must be able to fail without
// costing the rep any of it.
//
// Idempotent by replacement: a later pass over the same week is a correction,
// not an addition. The stamp moves with it.
//
// Empty prose stores as NULL with the stamp still written — the CHECK admits
// that deliberately, so a pass that ran and found the week unremarkable is
// distinguishable from one that never ran.
func (e *Engine) Narrate(ctx context.Context, reviewID ids.UUID, sentence string, now time.Time) error {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return err
	}
	userID, err := reviewUser(ctx)
	if err != nil {
		return err
	}
	// Bounded HERE as well as in the parser, because this is the writer: a
	// caller that reaches Narrate without going through narrative.Parse would
	// otherwise learn the ceiling from a driver error at 06:00 on a Monday.
	// Runes, because the column counts characters — a German sentence full of
	// umlauts is fewer characters than bytes.
	if n := len([]rune(sentence)); n > narrative.MaxNarrativeRunes {
		return httperr.Validation("narrative", "too_long",
			fmt.Sprintf("the sentence is %d characters, over the %d the column holds",
				n, narrative.MaxNarrativeRunes))
	}
	return database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		var before *string
		var beforeStamp *time.Time
		// The owner check is the row scope: a review belongs to the rep whose
		// week it was, and the id alone must not reach anybody else's.
		row := tx.QueryRow(ctx, `
			SELECT narrative, narrated_at FROM weekly_review
			 WHERE id = $1 AND user_id = $2
			 FOR UPDATE`, reviewID, userID)
		switch err := row.Scan(&before, &beforeStamp); {
		case errors.Is(err, pgx.ErrNoRows):
			return apperrors.ErrNotFound
		case err != nil:
			return err
		}

		stamp := now.UTC()
		var stored *string
		if sentence != "" {
			stored = &sentence
		}
		if _, err := tx.Exec(ctx,
			`UPDATE weekly_review SET narrative = $2, narrated_at = $3 WHERE id = $1`,
			reviewID, stored, stamp); err != nil {
			return err
		}
		_, err := storekit.Audit(ctx, tx, "update", "weekly_review", reviewID,
			map[string]any{"narrative": before, "narrated_at": beforeStamp},
			map[string]any{"narrative": stored, "narrated_at": stamp})
		return err
	})
}
