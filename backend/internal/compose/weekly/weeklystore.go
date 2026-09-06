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
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/weekly/learnings"
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

	// Outlook is where the week was landing, per horizon, read from the frozen
	// copies rather than from the snapshots they came from. EMPTY where no
	// forecast was composed when the review was written — which the panel says
	// rather than drawing zeros, because a week nobody forecast and a week that
	// landed on nothing are different facts.
	Outlook []Outlook

	// How well the week went, as against what happened in it. Nil when the
	// review predates the scorecard; each of its two blocks is independently
	// absent when the rep had no such work, which is a different fact from
	// every count in it being zero.
	Scorecard *Scorecard

	// What the week taught, and whether anybody looked.
	//
	// LearningsState is load-bearing beside the list: an empty list means the
	// pass found nothing it could ground, and `not_run` means no pass has
	// looked at all. A reader that drew both as "nothing to learn" would tell a
	// rep something the product never established.
	LearningsState string
	Learnings      []learnings.Learning
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
	// teams answers whether the caller belongs to the team they are asking
	// about. Nil FAILS CLOSED: a team read is refused rather than served
	// unchecked, because an unbound seam is a wiring mistake and serving the
	// snapshot anyway would hand every lead every team's week.
	teams TeamMembers
	// forecast freezes where the week was landing. Nil where no forecast is
	// composed — the review is then written WITHOUT an outlook rather than not
	// written, because a retrospective of what a rep did is worth having on an
	// installation that forecasts nothing.
	forecast ForecastWeek
}

// TeamMembers answers whether the caller belongs to a named team.
//
// The team-id half of the membership question identity already answers by user
// id for the Worklist's owner reads. It lives behind an interface for the
// reason WeekPlan does: this package cannot import the module that owns
// team_membership, and what it needs is one boolean.
type TeamMembers interface {
	// CallerLeadsLiveTeam reports whether the acting human is a live member of
	// this live team. False for a team that does not exist, so an outsider
	// cannot learn which team ids are real.
	CallerLeadsLiveTeam(ctx context.Context, team ids.UUID) (bool, error)
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

// ForecastWeek freezes a closed week's landing and how it got there.
//
// A seam because forecasting owns the arithmetic and the snapshots, and this
// package owns the review they are frozen into. The composition binds the two;
// neither module reaches for the other.
type ForecastWeek interface {
	// CloseWeek takes the closing snapshot for the week ending at weekEnd and
	// reports the horizons, the bars between the two snapshots, and the deals
	// behind each bar. Idempotent on the day: a retried job finds the snapshot
	// already taken and reports the same figures rather than freezing a second
	// one, which would give the next week two candidate openings.
	CloseWeek(ctx context.Context, tx pgx.Tx, weekStart, weekEnd time.Time) (
		[]Outlook, []Movement, []Driver, error)

	// CloseTeamWeek is the same over a TEAM's book. Not the sum of its
	// members': a deal owned by nobody on the team is in neither, and one the
	// team works but a member owns is in both, so adding six personal outlooks
	// would answer a question nobody asked.
	CloseTeamWeek(ctx context.Context, tx pgx.Tx, teamID ids.UUID, weekStart, weekEnd time.Time) (
		[]Outlook, []Movement, []Driver, error)
}

// NewEngine binds the engine to the installation pool and to the membership
// question its team reads are gated on.
//
// TeamMembers is an ARGUMENT rather than a With… option, unlike WeekPlan beside
// it, and the asymmetry is deliberate: an absent plan degrades honestly — the
// review counts no commitments and says so — while an absent membership seam
// refuses every team read. A caller that forgets an option gets a broken
// snapshot job at the second tick of the week; a caller that forgets an
// argument does not compile.
//
// Nil is still handled where the gate reads it, because a test may pass one to
// assert the refusal, and a gate that trusted the constructor would be a gate
// with a hole in it the day someone adds a second construction path.
func NewEngine(pool *pgxpool.Pool, teams TeamMembers) *Engine {
	return &Engine{pool: pool, teams: teams}
}

// WithPlan binds the week-ahead, so a closed review carries what the plan came
// to. Absent, the review's commitment counts stay zero.
func (e *Engine) WithPlan(plan WeekPlan) *Engine {
	e.plan = plan
	return e
}

// WithForecast binds the forecast, so a closed review freezes where the week
// was landing. Absent, the review carries no outlook and the panel says so.
func (e *Engine) WithForecast(forecast ForecastWeek) *Engine {
	e.forecast = forecast
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
