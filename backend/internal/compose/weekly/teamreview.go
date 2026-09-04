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
// as a third phase: a snapshot assembled while reps are still being measured
// would freeze a team that was half-counted, and nothing afterwards would say
// so.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The rules that pick a rep's one coaching focus, in the order they are tried.
//
// Ordered by what a lead should raise FIRST, not by severity in the abstract: a
// rep who asked for help has already said what they need, and walking past that
// to raise a metric is the fastest way to teach them not to ask.
const (
	// They asked. Nothing outranks a request already made.
	FocusHelpRequested = "help_requested"
	// A customer waited past the target and nobody answered.
	FocusLeadsBreached = "leads_breached"
	// They planned a week and did not keep it.
	FocusCommitmentsMissed = "commitments_missed"
	// Meetings happened and left nothing behind.
	FocusMeetingsWithoutNextStep = "meetings_without_next_step"
	// Nothing to fix, and something to copy. Without this a healthy rep
	// produces no row at all, and a page promising one focus per rep would
	// quietly shorten to the troubled ones — which reads as a team where only
	// those people exist.
	FocusStrongWeek = "strong_week"
	// Nothing to fix and nothing that stood out. Said plainly rather than
	// dressed as either a problem or a triumph.
	FocusQuietWeek = "quiet_week"
)

// TeamMember is one seat the snapshot should cover, as the caller knows it.
type TeamMember struct {
	UserID      ids.UUID
	DisplayName string
}

// TeamReview is one team's frozen week.
type TeamReview struct {
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

// TeamRep is one member's week as their lead reads it, with the one thing to
// raise.
type TeamRep struct {
	UserID          ids.UUID
	DisplayName     string
	DealsWon        int
	LeadsBreached   int
	MeetingsHeld    int
	CommitmentsDue  int
	CommitmentsKept int
	HelpRequested   int
	FocusKind       string
	FocusLabel      string
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
	var review TeamReview
	var created bool
	err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		thisWeek, err := WeekStartOf(ctx, tx, now)
		if err != nil {
			return err
		}
		week := thisWeek.AddDate(0, 0, -7)

		review = TeamReview{
			TeamID: teamID, TeamName: teamName,
			LocalWeekStart: week, AsOf: now.UTC(),
		}
		if err := gatherTeamWeek(ctx, tx, &review, members); err != nil {
			return err
		}
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
		return insertTeamReps(ctx, tx, id, review.Reps)
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

// gatherTeamWeek reads each member's frozen week and totals them.
//
// A member with NO review for the week is counted as unread rather than as a
// zero row: a rep who was on leave and one whose measurement failed look
// identical as zeros, and only one of those is a fact about their week.
func gatherTeamWeek(
	ctx context.Context, tx pgx.Tx, review *TeamReview, members []TeamMember,
) error {
	// Money is answerable only if EVERY member's week converted. One rep whose
	// currency had no rate makes the team total unanswerable, exactly as one
	// unconvertible deal does for that rep — a sum quietly missing a person is
	// worse than an absent one.
	money := Money{Known: true}
	for _, member := range members {
		counts, memberMoney, help, err := memberWeek(ctx, tx, member.UserID, review.LocalWeekStart)
		if errors.Is(err, apperrors.ErrNotFound) {
			review.RepsUnread++
			continue
		}
		if err != nil {
			return err
		}
		review.Counts.RepsCounted++
		addTeamCounts(&review.Counts, counts)
		if !memberMoney.Known {
			money.Known = false
		} else if money.Known {
			money.Currency = memberMoney.Currency
			money.CreatedMinor += memberMoney.CreatedMinor
			money.WonMinor += memberMoney.WonMinor
			money.LostMinor += memberMoney.LostMinor
		}
		review.Reps = append(review.Reps, repFrom(member, counts, help))
	}
	if money.Known && money.Currency != "" {
		review.Money = money
	}
	return nil
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

// repFrom builds one member's row, focus and all.
func repFrom(member TeamMember, counts Counts, help int) TeamRep {
	kind, label := focusFor(counts, help)
	return TeamRep{
		UserID: member.UserID, DisplayName: member.DisplayName,
		DealsWon: counts.DealsWon, LeadsBreached: counts.LeadsBreached,
		MeetingsHeld:   counts.MeetingsHeld,
		CommitmentsDue: counts.CommitmentsDue, CommitmentsKept: counts.CommitmentsKept,
		HelpRequested: help,
		FocusKind:     kind, FocusLabel: label,
	}
}

// focusFor picks the ONE thing a lead should raise with this rep.
//
// One per rep, always — including the rep whose week went well, whose focus is
// what the team should copy. A page promising one focus per rep and delivering
// rows only for the troubled ones reads as a team where only those people
// exist, which is both untrue and demoralising to be listed in.
//
// The label is composed here rather than by a model: it states a stored figure
// and nothing else, so it cannot say something the snapshot does not hold.
func focusFor(counts Counts, help int) (kind, label string) {
	switch {
	case help > 0:
		return FocusHelpRequested, fmt.Sprintf("Asked for help on %s", plural(help, "commitment"))
	case counts.LeadsBreached > 0:
		return FocusLeadsBreached, fmt.Sprintf("%s went past the response target",
			plural(counts.LeadsBreached, "lead"))
	case counts.CommitmentsDue > counts.CommitmentsKept:
		return FocusCommitmentsMissed, fmt.Sprintf("Kept %d of %d commitments",
			counts.CommitmentsKept, counts.CommitmentsDue)
	case counts.MeetingsHeld > counts.MeetingsWithNextStep:
		return FocusMeetingsWithoutNextStep, fmt.Sprintf("%s left without a next step",
			plural(counts.MeetingsHeld-counts.MeetingsWithNextStep, "meeting"))
	case counts.DealsWon > 0:
		return FocusStrongWeek, fmt.Sprintf("Won %s — worth asking how",
			plural(counts.DealsWon, "deal"))
	case counts.CommitmentsDue > 0 && counts.CommitmentsDue == counts.CommitmentsKept:
		return FocusStrongWeek, fmt.Sprintf("Kept every commitment (%d)", counts.CommitmentsDue)
	default:
		return FocusQuietWeek, "A quiet week"
	}
}

// plural renders a count with its noun, so a label reads as a sentence.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
