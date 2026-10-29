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

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
	       base_currency, reps_unread, numeric_summary,
	       proposals_accepted, proposals_rejected, auto_change_reversals, coalesce(summary, ''), coalesce(summary_language, 'en')
	  FROM team_weekly_review`

// memberFrozenWeek is one member's frozen week as the team snapshot folds it.
type memberFrozenWeek struct {
	counts Counts
	money  Money
	help   int
	// plans is the member review's commitments statement; nil where that week
	// settled no plan or was frozen before figures carried coverage.
	plans *crmcontracts.WeeklyFigureCoverage
}

// memberWeek reads one member's frozen week, and how many commitments they
// asked for help on.
//
// ErrNotFound when that rep has no review for the week — which is a fact about
// the snapshot ("this member's week was not counted"), not an error.
func memberWeek(
	ctx context.Context, tx pgx.Tx, userID ids.UUID, week time.Time,
) (memberFrozenWeek, error) {
	var c Counts
	var created, won, lost *int64
	var currency *string
	var numeric *crmcontracts.WeeklyNumericSummary
	err := tx.QueryRow(ctx, `
		SELECT deals_won, deals_lost, deals_moved,
		       leads_routed, leads_answered_in_target, leads_breached,
		       meetings_held, meetings_with_next_step,
		       commitments_due, commitments_kept,
		       pipeline_created_minor, pipeline_won_minor, pipeline_lost_minor, base_currency,
		       numeric_summary, proposals_accepted, proposals_rejected
		  FROM weekly_review WHERE user_id = $1 AND local_week_start = $2`, userID, week).
		Scan(&c.DealsWon, &c.DealsLost, &c.DealsMoved,
			&c.LeadsRouted, &c.LeadsAnsweredInTarget, &c.LeadsBreached,
			&c.MeetingsHeld, &c.MeetingsWithNextStep,
			&c.CommitmentsDue, &c.CommitmentsKept,
			&created, &won, &lost, &currency, &numeric,
			&c.ProposalsAccepted, &c.ProposalsRejected)
	if errors.Is(err, pgx.ErrNoRows) {
		return memberFrozenWeek{}, apperrors.ErrNotFound
	}
	if err != nil {
		return memberFrozenWeek{}, fmt.Errorf("weekly: reading a member's week: %w", err)
	}
	out := memberFrozenWeek{counts: c}
	if currency != nil {
		out.money = Money{
			CreatedMinor: deref(created), WonMinor: deref(won), LostMinor: deref(lost),
			Currency: *currency, Known: true,
		}
	}
	if numeric != nil && numeric.FigureCoverage != nil {
		out.plans = numeric.FigureCoverage.Commitments
	}
	// How many of that week's commitments carried a request for help. Counted
	// from the plan rather than the review, because the review freezes what the
	// week came to and not what was asked along the way.
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM weekly_plan_commitment c
		  JOIN weekly_plan p ON p.id = c.plan_id
		 WHERE p.owner_id = $1 AND p.local_week_start = $2
		   AND btrim(c.help_requested) <> ''
		   AND btrim(coalesce(c.manager_response, '')) = ''
		   AND c.state NOT IN ('done', 'dropped')`, userID, week).Scan(&out.help); err != nil {
		return memberFrozenWeek{}, fmt.Errorf("weekly: counting a member's requests: %w", err)
	}
	return out, nil
}

// mayReadTeam decides whether this caller may open this team's week.
//
// auth.TeamWeekReachOf says how far the caller reaches: a seat holding the
// oversight grant opens any team, a coaching seat opens a team it is on, and
// every other seat is refused. Row scope does not admit anyone here — a
// read-only seat reaches every row and leads nobody.
//
// The membership question stays because the team id arrives from the request
// and nothing else narrows the row: without it a lead of one team reads any
// other team's week by changing one query parameter.
//
// The weekly job re-reads the snapshot it just wrote under a MEMBER's own
// authority, so its engine needs the seam bound like any reader's.
//
// Two answers, deliberately different. A seat that may open no team's week gets
// ErrPermissionDenied, which the screen draws as a refusal. A lead asking about
// a team they are not on gets ErrNotFound: telling "this team exists but is not
// yours" apart from "no such team" would let them enumerate the chart one id at
// a time, the same reason a row-scope miss reads as 404 everywhere else.
func (e *Engine) mayReadTeam(ctx context.Context, teamID ids.UUID) error {
	if err := e.requireIndividualPerformance(ctx); err != nil {
		return err
	}
	switch auth.TeamWeekReachOf(ctx) {
	case auth.ReachesEveryTeam:
		return nil
	case auth.ReachesTeamsLed:
		return e.mayReadTeamLed(ctx, teamID)
	default:
		return apperrors.ErrPermissionDenied
	}
}

// requireIndividualPerformance refuses every team's week while the installation shows no
// individual performance, a week frozen before the switch included.
func (e *Engine) requireIndividualPerformance(ctx context.Context) error {
	allowed, bound := auth.IndividualPerformanceOf(ctx)
	if !bound {
		err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
			var err error
			allowed, err = identity.IndividualPerformanceTx(ctx, tx)
			return err
		})
		if err != nil {
			return fmt.Errorf("weekly: reading whether individual performance is shown: %w", err)
		}
	}
	if !allowed {
		return auth.IndividualPerformanceOffError{}
	}
	return nil
}

// mayReadTeamLed asks whether a lead is on this team.
//
// Fails closed. An unbound seam is a wiring mistake, and serving the snapshot
// anyway would hand every lead every team's week.
func (e *Engine) mayReadTeamLed(ctx context.Context, teamID ids.UUID) error {
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
		review, err = e.readTeamReview(ctx, tx, row)
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
		review, err = e.readTeamReview(ctx, tx, row)
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
		&created, &won, &lost, &currency, &review.RepsUnread, &review.NumericSummary,
		&review.Quality.ProposalsAccepted, &review.Quality.ProposalsRejected,
		&review.Quality.AutoChangeReversals, &review.Summary, &review.Language); {
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
	// Filled here because BOTH readers — TeamReview and LatestTeamReview — come
	// through this scan. Filled at either call site instead, one surface would
	// draw the team's landing and the other would not.
	//
	// A snapshot written before the outlook existed simply has no rows, which
	// reads as an empty list and draws "no forecast" rather than failing.
	if review.Outlook, err = readOutlookFrom(ctx, tx, teamOutlook, review.ID); err != nil {
		return TeamReview{}, err
	}
	if review.Members, err = readTeamMembers(ctx, tx, review.ID); err != nil {
		return TeamReview{}, err
	}
	if review.Coverage, err = readTeamCoverage(ctx, tx, review.ID); err != nil {
		return TeamReview{}, err
	}
	return review, nil
}

// readTeamReview is the frozen snapshot plus what needs attention now.
func (e *Engine) readTeamReview(ctx context.Context, tx pgx.Tx, row pgx.Row) (TeamReview, error) {
	review, err := scanTeamReview(ctx, tx, row)
	if err != nil {
		return TeamReview{}, err
	}
	withheldFromReader(ctx, &review)
	if err := e.readTeamAttention(ctx, tx, &review); err != nil {
		return TeamReview{}, err
	}
	return review, nil
}
