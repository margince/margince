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
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
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

	// What the week did to the pipeline, in the installation's base currency.
	//
	// Beside Counts rather than inside it because a tally and a conversion fail
	// differently: every count is always answerable, and this one has a third
	// outcome — not computable — that a tally has no room for.
	Money Money

	// The review this week is measured against: the same rep's most recent
	// earlier one, or nil for their first.
	//
	// The deltas themselves are NOT stored: a stored difference is a third copy
	// of what the two rows already say, and the three drift the first time one
	// row is rewritten.
	//
	// Held by: TestNoDeltaIsStoredBesideTheTwoFrozenRows
	// (backend/internal/compose/weekly/weeklycompare_integration_test.go)
	PriorReviewID *ids.UUID
	// Prior is that row's own frozen figures, loaded for a reader that wants
	// the comparison. Nil when there is no prior week, and nil on the prior
	// row itself: a review carries ONE step of history, not a chain that
	// lengthens every week a rep works here.
	Prior *PriorWeek
}

// PriorWeek is the earlier review a week is compared against.
type PriorWeek struct {
	LocalWeekStart time.Time
	Counts         Counts
	Money          Money
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

	// How the week's inbound leads were answered. A lead still inside its
	// target when the row was written is in neither of the last two: it has
	// not yet been either answered in time or missed.
	// What the week's PLAN came to. Read through a seam rather than queried
	// here: weeklyplan owns those tables, and a review that counted them
	// itself would be a second reader of somebody else's rows.
	CommitmentsDue  int
	CommitmentsKept int

	LeadsRouted           int
	LeadsAnsweredInTarget int
	LeadsBreached         int

	// Meetings held, and how many left a next step behind. The second is the
	// one a rep can act on.
	MeetingsHeld         int
	MeetingsWithNextStep int
}

// Money is a base-currency figure, or the honest absence of one.
//
// The three totals travel together with their currency because they are one
// answer: either the week's pipeline movement converted, or it did not. A
// caller cannot be handed two of the three and left to guess about the rest.
type Money struct {
	CreatedMinor int64
	WonMinor     int64
	LostMinor    int64
	Currency     string
	// Known is false when a deal in the week could not be converted, or when
	// the installation names no base currency. The zero value is therefore the
	// honest "not computable", not a week in which nothing happened.
	Known bool
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
type Engine struct {
	pool *pgxpool.Pool
	// plan settles the rep's week-ahead and reports what it came to. Nil where
	// no plan module is bound — the review then counts no commitments rather
	// than failing, because a retrospective is still worth having without one.
	plan WeekPlan
}

// WeekPlan settles the closing week's plan and says what it came to.
//
// The one edge between the retrospective and the plan, in ONE direction. This
// package cannot import the module that owns those tables, and would not want
// to: what it needs is two integers, and asking for them through an interface
// is what keeps the plan's rows the plan's business.
type WeekPlan interface {
	// CloseWeek settles the caller's plan for the week that closed and returns
	// how many commitments were owed and how many kept. Idempotent: a second
	// call over a settled week answers the same figures without moving a row.
	CloseWeek(ctx context.Context, now time.Time) (due, kept int, err error)
}

// NewEngine binds the engine to the installation pool.
func NewEngine(pool *pgxpool.Pool) *Engine { return &Engine{pool: pool} }

// WithPlan binds the week-ahead, so a closed review carries what the plan came
// to. Absent, the review's commitment counts stay zero.
func (e *Engine) WithPlan(plan WeekPlan) *Engine {
	e.plan = plan
	return e
}

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
