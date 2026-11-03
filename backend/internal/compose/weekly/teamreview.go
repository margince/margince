// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// The team's week, assembled from the member weeks and frozen beside them.
//
// A LEAD's read, and a different question from the board in attention: that one
// answers what the team is carrying NOW, live and recomputed on every open.
// This answers what last week WAS, once, so two weeks can be compared and
// neither moves under the comparison.
//
// It reads only reviews that are already written, which is why the job runs it
// after individual measurement: a snapshot assembled while reps are still being measured
// would freeze a team that was half-counted, and nothing afterwards would say
// so.

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TeamMember is one seat the snapshot should cover, as the caller knows it.
type TeamMember struct {
	UserID      ids.UUID
	DisplayName string
}

// TeamReview is one team's frozen week.
type TeamReview struct {
	NumericSummary *crmcontracts.WeeklyNumericSummary
	ID             ids.UUID
	TeamID         ids.UUID
	TeamName       string
	LocalWeekStart time.Time
	GeneratedAt    time.Time
	AsOf           time.Time
	Counts         TeamCounts
	Money          Money
	// RepsUnread counts the members whose week could not be read at all. Zero
	// is the claim that every member's week was counted.
	RepsUnread int
	Reps       []TeamRep

	// Outlook is where the team's week was landing, per horizon, frozen at
	// close. EMPTY when no forecast was composed — which is not the same as a
	// team that landed on nothing, so a reader says "no forecast" rather than
	// drawing zeros.
	Outlook []Outlook

	// Members are the week's live members, measured or not. Empty on a
	// snapshot frozen before the roster was kept.
	Members []FrozenMember
	Quality TeamQuality
	// Coverage is the month's and the quarter's, empty where no forecast was
	// composed or the team's book was refused.
	Coverage []Coverage
	// Summary is the paragraph at the top of the review, composed from the
	// figures above by fixed rules. Empty on an older snapshot.
	Summary string
	// Language keeps permission-filtered summaries in the frozen record language.
	Language string

	// Attention is computed when the review is read, never frozen: what needs
	// the manager now, over the frozen roster.
	Attention TeamAttentionItems
	// MeetingsWithheld is Review.MeetingsWithheld for the team's week.
	MeetingsWithheld bool
}

// FrozenMember is one seat on the team that week, as it was named then.
type FrozenMember struct {
	UserID      ids.UUID
	DisplayName string
	// Measured says the member's own week was read into the counts.
	Measured bool
}

// TeamQuality is how well the team kept the CRM over the week. Each figure is
// nil on a snapshot frozen before it was measured.
type TeamQuality struct {
	ProposalsAccepted   *int
	ProposalsRejected   *int
	AutoChangeReversals *int
}

// TeamCounts are the team's totals over its member weeks.
type TeamCounts struct {
	RepsCounted int
	DealsWon    int
	DealsLost   int
	// DealsMoved counts deals that changed stage without closing — the week's
	// advancement. Every member's review has carried it since the review
	// shipped; the team total dropped it, so a team that moved eleven deals and
	// closed none read as a team that did nothing.
	DealsMoved            int
	LeadsRouted           int
	LeadsAnsweredInTarget int
	LeadsBreached         int
	MeetingsHeld          int
	MeetingsWithNextStep  int
	CommitmentsDue        int
	CommitmentsKept       int
}

// TeamRep is one member's week as their lead reads it, with what to raise.
type TeamRep struct {
	UserID          ids.UUID
	DisplayName     string
	DealsWon        int
	LeadsBreached   int
	MeetingsHeld    int
	CommitmentsDue  int
	CommitmentsKept int
	HelpRequested   int
	// FocusKind and FocusLabel are always Patterns[0].
	FocusKind  string
	FocusLabel string
	Patterns   []Pattern
	Funnel     []FunnelDeviation
}

// AssembleTeamFor writes the team's snapshot for the week that closed, or
// answers the one already written.
//
// Idempotent by the unique constraint, like the per-rep review: the dispatcher
// ticks more than once inside a week, and a second run must not rewrite a
// snapshot whose numbers a lead has already read.
func (e *Engine) AssembleTeamFor(
	ctx context.Context, teamID ids.UUID, teamName string, members []TeamMember, now time.Time,
) (TeamReview, bool, error) {
	if err := e.mayReadTeam(ctx, teamID); err != nil {
		return TeamReview{}, false, err
	}
	var review TeamReview
	var created bool
	err := e.withMeasurementSnapshot(ctx, func(tx pgx.Tx) error {
		if err := refuseWhileIndividualOff(ctx, tx); err != nil {
			return err
		}
		thisWeek, err := WeekStartOf(ctx, tx, now)
		if err != nil {
			return err
		}
		week := thisWeek.AddDate(0, 0, -7)

		review = TeamReview{
			TeamID: teamID, TeamName: teamName,
			LocalWeekStart: week, AsOf: now.UTC(),
		}
		var measured bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM team_weekly_review WHERE team_id=$1 AND local_week_start=$2 AND reps_counted>0)`, teamID, week).Scan(&measured); err != nil {
			return err
		}
		if measured {
			return nil
		}
		review.Language = string(teamCopyFor(string(identity.BaseLanguageForRecord(ctx, tx))).Lang())
		// Every figure is in memory before the parent row is written. The
		// summary is built from all of them and travels in that one INSERT.
		if err := e.measureTeamWeek(ctx, tx, &review, members, now); err != nil {
			return err
		}
		review.Summary = summarise(review)
		id, wrote, err := insertTeamReview(ctx, tx, review)
		if err != nil {
			return err
		}
		created = wrote
		if !wrote {
			// Somebody already wrote this team's week. Their snapshot stands.
			return nil
		}
		review.ID = id
		return insertTeamChildren(ctx, tx, review)
	})
	if err != nil {
		return TeamReview{}, false, err
	}
	if !created {
		existing, err := e.TeamReview(ctx, teamID, review.LocalWeekStart)
		return existing, false, err
	}
	return review, true, nil
}

// measureTeamWeek computes everything the snapshot freezes.
func (e *Engine) measureTeamWeek(
	ctx context.Context, tx pgx.Tx, review *TeamReview, members []TeamMember, now time.Time,
) error {
	plans, err := gatherTeamWeek(ctx, tx, review, members)
	if err != nil {
		return err
	}
	if err := e.measureTeamNumeric(ctx, tx, review, members, plans, now); err != nil {
		return err
	}
	start, end, err := localWeekWindow(ctx, tx, review.LocalWeekStart)
	if err != nil {
		return err
	}
	reversals, err := countAutoChangeReversals(ctx, tx, review.Members, start, end)
	if err != nil {
		return err
	}
	review.Quality.AutoChangeReversals = &reversals
	if err := measureTeamFunnel(ctx, tx, review, end); err != nil {
		return err
	}
	return e.closeTeamForecast(ctx, tx, review, start, end)
}

// closeTeamForecast freezes where the team's week was landing and whether its
// pipeline covered the month and the quarter.
//
// A refused landing costs the landing, not the week. Forecasting refuses
// ScopeTeam to a caller outside the team; letting that escape rolls back the
// whole retrospective. Only a refusal is absorbed: a broken query still fails.
//
// Held by: TestARefusedTeamOutlookStillLeavesTheWeekWritten
// (backend/internal/compose/integration/teamoutlook_integration_test.go)
func (e *Engine) closeTeamForecast(
	ctx context.Context, tx pgx.Tx, review *TeamReview, start, end time.Time,
) error {
	if e.forecast == nil {
		return nil
	}
	closed, err := e.forecast.CloseTeamWeek(ctx, tx, review.TeamID, start, end)
	switch {
	case errors.Is(err, apperrors.ErrNotFound),
		errors.Is(err, apperrors.ErrPermissionDenied):
		return nil
	case err != nil:
		return err
	}
	review.Outlook = closed.Outlooks
	review.Coverage = closed.Coverage
	return nil
}

// gatherTeamWeek reads each member's frozen week and totals them.
//
// A member with no review for the week is counted as unread rather than as a
// zero row. A rep on leave and one whose measurement failed look identical as
// zeros, and only one is a fact about their week. They still join the roster,
// marked unmeasured.
func gatherTeamWeek(
	ctx context.Context, tx pgx.Tx, review *TeamReview, members []TeamMember,
) (memberPlans, error) {
	// Money is answerable only if EVERY member's week converted. One rep whose
	// currency had no rate makes the team total unanswerable, exactly as one
	// unconvertible deal does for that rep — a sum quietly missing a contact is
	// worse than an absent one.
	money := Money{Known: true}
	var plans memberPlans
	var accepted, rejected int
	for _, member := range members {
		frozen, err := memberWeek(ctx, tx, member.UserID, review.LocalWeekStart)
		if errors.Is(err, apperrors.ErrNotFound) {
			review.RepsUnread++
			money.Known = false
			review.Members = append(review.Members, FrozenMember{UserID: member.UserID, DisplayName: member.DisplayName})
			continue
		}
		if err != nil {
			return memberPlans{}, err
		}
		review.Members = append(review.Members, FrozenMember{
			UserID: member.UserID, DisplayName: member.DisplayName, Measured: true,
		})
		counts, memberMoney := frozen.counts, frozen.money
		plans.add(frozen.plans)
		review.Counts.RepsCounted++
		addTeamCounts(&review.Counts, counts)
		accepted += counts.ProposalsAccepted
		rejected += counts.ProposalsRejected
		if !memberMoney.Known || (money.Currency != "" && money.Currency != memberMoney.Currency) {
			money.Known = false
		} else if money.Known {
			money.Currency = memberMoney.Currency
			money.CreatedMinor += memberMoney.CreatedMinor
			money.WonMinor += memberMoney.WonMinor
			money.LostMinor += memberMoney.LostMinor
		}
		deals, err := memberDealBlock(ctx, tx, member.UserID, review.LocalWeekStart)
		if err != nil {
			return memberPlans{}, err
		}
		review.Reps = append(review.Reps, teamCopyFor(review.Language).repFrom(member, counts, frozen.help, deals))
	}
	review.Quality.ProposalsAccepted, review.Quality.ProposalsRejected = &accepted, &rejected
	if money.Known && money.Currency != "" {
		review.Money = money
	}
	return plans, nil
}

// addTeamCounts folds one member's week into the team's totals.
func addTeamCounts(team *TeamCounts, member Counts) {
	team.DealsWon += member.DealsWon
	team.DealsLost += member.DealsLost
	team.DealsMoved += member.DealsMoved
	team.LeadsRouted += member.LeadsRouted
	team.LeadsAnsweredInTarget += member.LeadsAnsweredInTarget
	team.LeadsBreached += member.LeadsBreached
	team.MeetingsHeld += member.MeetingsHeld
	team.MeetingsWithNextStep += member.MeetingsWithNextStep
	team.CommitmentsDue += member.CommitmentsDue
	team.CommitmentsKept += member.CommitmentsKept
}

// repFrom builds one member's row, patterns and all.
func (said teamCopy) repFrom(member TeamMember, counts Counts, help int, deals *DealBlock) TeamRep {
	patterns := said.patternsFor(counts, help, deals)
	return TeamRep{
		UserID: member.UserID, DisplayName: member.DisplayName,
		DealsWon: counts.DealsWon, LeadsBreached: counts.LeadsBreached,
		MeetingsHeld:   counts.MeetingsHeld,
		CommitmentsDue: counts.CommitmentsDue, CommitmentsKept: counts.CommitmentsKept,
		HelpRequested: help,
		FocusKind:     patterns[0].Kind, FocusLabel: patterns[0].Label,
		Patterns: patterns,
	}
}
