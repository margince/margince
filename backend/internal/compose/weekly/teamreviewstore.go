// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// Reading and writing the team's frozen week.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// teamReviewSelect is the snapshot's columns, in the order scanTeamReview reads
// them. ONE spelling, for the reason reviewSelect is one: two copies is how the
// same week comes to render two different ways.
const teamReviewSelect = `
	SELECT id, team_id, team_name, local_week_start, generated_at, as_of,
	       reps_counted, deals_won, deals_lost, deals_moved,
	       leads_routed, leads_answered_in_target, leads_breached,
	       meetings_held, meetings_with_next_step,
	       commitments_due, commitments_kept,
	       pipeline_created_minor, pipeline_won_minor, pipeline_lost_minor,
	       base_currency, reps_unread
	  FROM team_weekly_review`

// memberWeek reads one member's frozen week, and how many commitments they
// asked for help on.
//
// ErrNotFound when that rep has no review for the week — which is a fact about
// the snapshot ("this member's week was not counted"), not an error.
func memberWeek(
	ctx context.Context, tx pgx.Tx, userID ids.UUID, week time.Time,
) (Counts, Money, int, error) {
	var c Counts
	var created, won, lost *int64
	var currency *string
	err := tx.QueryRow(ctx, `
		SELECT deals_won, deals_lost, deals_moved,
		       leads_routed, leads_answered_in_target, leads_breached,
		       meetings_held, meetings_with_next_step,
		       commitments_due, commitments_kept,
		       pipeline_created_minor, pipeline_won_minor, pipeline_lost_minor, base_currency
		  FROM weekly_review WHERE user_id = $1 AND local_week_start = $2`, userID, week).
		Scan(&c.DealsWon, &c.DealsLost, &c.DealsMoved,
			&c.LeadsRouted, &c.LeadsAnsweredInTarget, &c.LeadsBreached,
			&c.MeetingsHeld, &c.MeetingsWithNextStep,
			&c.CommitmentsDue, &c.CommitmentsKept,
			&created, &won, &lost, &currency)
	if errors.Is(err, pgx.ErrNoRows) {
		return Counts{}, Money{}, 0, apperrors.ErrNotFound
	}
	if err != nil {
		return Counts{}, Money{}, 0, fmt.Errorf("weekly: reading a member's week: %w", err)
	}
	money := Money{}
	if currency != nil {
		money = Money{
			CreatedMinor: deref(created), WonMinor: deref(won), LostMinor: deref(lost),
			Currency: *currency, Known: true,
		}
	}
	// How many of that week's commitments carried a request for help. Counted
	// from the plan rather than the review, because the review freezes what the
	// week came to and not what was asked along the way.
	var help int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM weekly_plan_commitment c
		  JOIN weekly_plan p ON p.id = c.plan_id
		 WHERE p.owner_id = $1 AND p.local_week_start = $2
		   AND btrim(c.help_requested) <> ''`, userID, week).Scan(&help); err != nil {
		return Counts{}, Money{}, 0, fmt.Errorf("weekly: counting a member's requests: %w", err)
	}
	return c, money, help, nil
}

// insertTeamReview writes the snapshot unless this team already has one for the
// week, reporting which of the two happened.
func insertTeamReview(ctx context.Context, tx pgx.Tx, review TeamReview) (ids.UUID, bool, error) {
	c := review.Counts
	cols, args := insertColumns{}, []any(nil)
	add := func(name string, value any) {
		cols = append(cols, name)
		args = append(args, value)
	}
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
	// The four money columns go together or not at all — the table's own CHECK
	// says a figure names its currency and a currency names figures.
	if review.Money.Known {
		add("pipeline_created_minor", review.Money.CreatedMinor)
		add("pipeline_won_minor", review.Money.WonMinor)
		add("pipeline_lost_minor", review.Money.LostMinor)
		add("base_currency", review.Money.Currency)
	}

	var id ids.UUID
	err := tx.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO team_weekly_review (%s) VALUES (%s)
		ON CONFLICT ON CONSTRAINT uq_team_weekly_review_team_week DO NOTHING
		RETURNING id`, cols.names(), cols.placeholders()), args...).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		// The team's week already had a snapshot. The loser of that race is not
		// an error: the constraint did its job.
		return ids.Nil, false, nil
	}
	if err != nil {
		return ids.Nil, false, fmt.Errorf("weekly: writing the team's week: %w", err)
	}
	return id, true, nil
}

// insertTeamReps writes the frozen membership.
func insertTeamReps(ctx context.Context, tx pgx.Tx, reviewID ids.UUID, reps []TeamRep) error {
	for _, rep := range reps {
		if _, err := tx.Exec(ctx, `
			INSERT INTO team_weekly_review_rep
			    (team_weekly_review_id, user_id, display_name, deals_won, leads_breached,
			     meetings_held, commitments_due, commitments_kept, help_requested,
			     focus_kind, focus_label)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			reviewID, rep.UserID, rep.DisplayName, rep.DealsWon, rep.LeadsBreached,
			rep.MeetingsHeld, rep.CommitmentsDue, rep.CommitmentsKept, rep.HelpRequested,
			rep.FocusKind, rep.FocusLabel); err != nil {
			return fmt.Errorf("weekly: writing a member's row: %w", err)
		}
	}
	return nil
}

// mayReadTeam decides whether this caller may open this team's week.
//
// TWO QUESTIONS, and both have to be asked. The row scope says whether a team
// snapshot is a question this reader may ask at all — an own-scoped reader
// would get a page about people whose rows they cannot read. Membership says
// WHICH team, and without it a lead of one team reads any other team's week by
// changing one query parameter, because the team id arrives from the request
// and nothing else narrows the row.
//
// A reader who sees every row passes without the membership question, matching
// how attention/scope.go resolves an owner: a management seat reaches every row
// by definition, and asking membership of it would refuse a reader the
// row-scope predicate then admits.
//
// It is NOT what lets the weekly job re-read the snapshot it just wrote: that
// job composes under a MEMBER's own authority rather than the system principal,
// deliberately, so its re-read passes the membership question like any other
// team-scoped reader. Its engine therefore needs the seam bound.
//
// An out-of-team lead gets ErrNotFound, not ErrPermissionDenied. A refusal that
// distinguished "this team exists but is not yours" from "no such team" would
// let an outsider enumerate the chart one id at a time — the same reason a
// row-scope miss reads as 404 everywhere else in this tree.
func (e *Engine) mayReadTeam(ctx context.Context, teamID ids.UUID) error {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Permissions.RowScope == principal.RowScopeOwn {
		return apperrors.ErrPermissionDenied
	}
	// auth.Unbounded rather than a RowScopeAll comparison of its own: it is the
	// tree's one spelling of "sees every row", and it also admits the system
	// principal — which the weekly job composes under when it re-reads a
	// snapshot it has just written to answer idempotently.
	if auth.Unbounded(actor) {
		return nil
	}
	// Fails closed. An unbound seam is a wiring mistake, and serving the
	// snapshot anyway would hand every lead every team's week.
	if e.teams == nil {
		return apperrors.ErrNotFound
	}
	member, err := e.teams.CallerLeadsLiveTeam(ctx, teamID)
	if err != nil {
		return err
	}
	if !member {
		return apperrors.ErrNotFound
	}
	return nil
}

// TeamReview answers one team's frozen week.
func (e *Engine) TeamReview(
	ctx context.Context, teamID ids.UUID, week time.Time,
) (TeamReview, error) {
	if err := e.mayReadTeam(ctx, teamID); err != nil {
		return TeamReview{}, err
	}
	var review TeamReview
	err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, teamReviewSelect+`
			 WHERE team_id = $1 AND local_week_start = $2`, teamID, week)
		var err error
		review, err = scanTeamReview(ctx, tx, row)
		return err
	})
	if err != nil {
		return TeamReview{}, err
	}
	return review, nil
}

// LatestTeamReview answers a team's most recent frozen week, or a named one.
//
// Same gate as TeamReview, and the same reason it is a separate entry point
// rather than a nil week on that one: "the newest" and "this one" are different
// questions, and a caller passing a zero time by accident would silently get
// the newest instead of a refusal.
func (e *Engine) LatestTeamReview(
	ctx context.Context, teamID ids.UUID, week *time.Time,
) (TeamReview, error) {
	if week != nil {
		return e.TeamReview(ctx, teamID, *week)
	}
	if err := e.mayReadTeam(ctx, teamID); err != nil {
		return TeamReview{}, err
	}
	var review TeamReview
	err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, teamReviewSelect+`
			 WHERE team_id = $1 ORDER BY local_week_start DESC LIMIT 1`, teamID)
		var err error
		review, err = scanTeamReview(ctx, tx, row)
		return err
	})
	if err != nil {
		return TeamReview{}, err
	}
	return review, nil
}

// scanTeamReview reads one snapshot row and its frozen membership.
func scanTeamReview(ctx context.Context, tx pgx.Tx, row pgx.Row) (TeamReview, error) {
	var review TeamReview
	c := &review.Counts
	var created, won, lost *int64
	var currency *string
	switch err := row.Scan(&review.ID, &review.TeamID, &review.TeamName,
		&review.LocalWeekStart, &review.GeneratedAt, &review.AsOf,
		&c.RepsCounted, &c.DealsWon, &c.DealsLost, &c.DealsMoved,
		&c.LeadsRouted, &c.LeadsAnsweredInTarget, &c.LeadsBreached,
		&c.MeetingsHeld, &c.MeetingsWithNextStep,
		&c.CommitmentsDue, &c.CommitmentsKept,
		&created, &won, &lost, &currency, &review.RepsUnread); {
	case errors.Is(err, pgx.ErrNoRows):
		return TeamReview{}, apperrors.ErrNotFound
	case err != nil:
		return TeamReview{}, err
	}
	if currency != nil {
		review.Money = Money{
			CreatedMinor: deref(created), WonMinor: deref(won), LostMinor: deref(lost),
			Currency: *currency, Known: true,
		}
	}
	reps, err := readTeamReps(ctx, tx, review.ID)
	if err != nil {
		return TeamReview{}, err
	}
	review.Reps = reps
	return review, nil
}

// readTeamReps reads the frozen membership, ordered as the page draws it.
//
// It joins NOTHING. Every word was written when the snapshot was, so a rep
// renamed or deleted since leaves the row exactly as the week recorded it.
func readTeamReps(ctx context.Context, tx pgx.Tx, reviewID ids.UUID) ([]TeamRep, error) {
	rows, err := tx.Query(ctx, `
		SELECT user_id, display_name, deals_won, leads_breached, meetings_held,
		       commitments_due, commitments_kept, help_requested, focus_kind, focus_label
		  FROM team_weekly_review_rep
		 WHERE team_weekly_review_id = $1
		 ORDER BY display_name, user_id`, reviewID)
	if err != nil {
		return nil, fmt.Errorf("weekly: reading the team's members: %w", err)
	}
	defer rows.Close()
	var out []TeamRep
	for rows.Next() {
		var rep TeamRep
		if err := rows.Scan(&rep.UserID, &rep.DisplayName, &rep.DealsWon,
			&rep.LeadsBreached, &rep.MeetingsHeld, &rep.CommitmentsDue,
			&rep.CommitmentsKept, &rep.HelpRequested,
			&rep.FocusKind, &rep.FocusLabel); err != nil {
			return nil, err
		}
		out = append(out, rep)
	}
	return out, rows.Err()
}
