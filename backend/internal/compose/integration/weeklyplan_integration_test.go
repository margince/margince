// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration_test

// The plan over real migrated Postgres.
//
// HERE rather than in the module, because it drives the compose harness and a
// module may never import the composition layer — depguard says so, and every
// other module test needing a seeded workspace already lives beside the
// harness for the same reason.
//
// What these defend is the access model and the settle, neither of which a unit
// test can see: a plan is one person's, its second reader is gated on a live
// team, and the week's close is idempotent because the dispatcher ticks more
// than once inside a week.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/weeklyplan"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// teammates is the membership seam, over the REAL identity service.
//
// A stub here would prove the store calls something; it would not prove the
// shipped gate admits a teammate and refuses an outsider, which is the whole
// question these tests exist to settle. The id conversion is the same one
// compose's own seam does.
type teammates struct{ svc *identity.Service }

func (a teammates) SharesLiveTeamWithCaller(ctx context.Context, other ids.UUID) (bool, error) {
	return a.svc.SharesLiveTeamWithCaller(ctx, ids.From[ids.UserKind](other))
}

// planClock is a Wednesday, so "this week" and "the week that closed" are
// unambiguous and neither is the fixture's own.
var planClock = time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)

type planEnv struct {
	*integration.Env
	store *weeklyplan.Store
	// rep1 and rep2 share a team; rep3 sits in another.
	rep1Ctx, rep2Ctx, rep3Ctx context.Context
}

func setupPlan(t *testing.T) *planEnv {
	t.Helper()
	e := integration.Setup(t)
	// The same handle compose builds (InstallationDB): the pool bound to the
	// installation's workspace resolver. A raw pool would skip the workspace
	// GUC every tenant query runs under.
	svc := identity.NewService(e.Pool)
	db := database.Bind(e.Pool, svc.InstallationWorkspace)
	return &planEnv{
		Env:     e,
		store:   weeklyplan.NewStore(db, weekly.WeekStartOf, teammates{svc: svc}),
		rep1Ctx: e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms),
		rep2Ctx: e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms),
		rep3Ctx: e.As(e.Rep3, []ids.UUID{e.Team2}, integration.AdminPerms),
	}
}

// A plan is one person's. Another rep's is not a thing this caller may read,
// and the answer is NOT FOUND rather than a refusal: whether a colleague has
// planned their week is itself something a stranger may not learn.
func TestAnotherRepsPlanIsNotFound(t *testing.T) {
	e := setupPlan(t)

	if _, err := e.store.StartWeek(e.rep1Ctx, planClock); err != nil {
		t.Fatalf("rep1 could not open their week: %v", err)
	}

	// rep3 is on another team entirely.
	_, err := e.store.PlanFor(e.rep3Ctx, e.Rep1, planClock)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("an outsider reading rep1's plan got %v, wanted not found", err)
	}
}

// The lead's read is the one path that names a person, and it is gated on the
// shared live team — the same question that decides whether a lead may open a
// rep's queue.
func TestATeammateReadsThePlanAndAnOutsiderDoesNot(t *testing.T) {
	e := setupPlan(t)

	if _, err := e.store.StartWeek(e.rep1Ctx, planClock); err != nil {
		t.Fatal(err)
	}

	if _, err := e.store.PlanFor(e.rep2Ctx, e.Rep1, planClock); err != nil {
		t.Errorf("a teammate could not read the plan: %v — without this the "+
			"refusal below would pass on a gate that admits nobody", err)
	}
	if _, err := e.store.PlanFor(e.rep3Ctx, e.Rep1, planClock); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("an outsider got %v, wanted not found", err)
	}
}

// The lead's ONE write, and the only path that touches somebody else's row.
func TestALeadAnswersARequestAndAnOutsiderCannot(t *testing.T) {
	e := setupPlan(t)

	commitment := planCommitment(t, e, e.rep1Ctx, "Call the Weber buyer")
	if err := e.store.AskForHelp(e.rep1Ctx, commitment, "their legal team will not answer"); err != nil {
		t.Fatal(err)
	}

	if err := e.store.Respond(e.rep3Ctx, commitment, "try again"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("an outsider answering got %v, wanted not found", err)
	}
	if err := e.store.Respond(e.rep2Ctx, commitment, "I will call their counsel"); err != nil {
		t.Fatalf("the teammate lead could not answer: %v", err)
	}

	plan, err := e.store.Current(e.rep1Ctx, planClock)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Commitments) != 1 {
		t.Fatalf("the plan came back with %d commitments", len(plan.Commitments))
	}
	got := plan.Commitments[0]
	if got.ManagerResponse != "I will call their counsel" {
		t.Errorf("the answer read back as %q", got.ManagerResponse)
	}
	// The three columns move together, because an answer with nobody behind it
	// cannot be shown to the person who asked.
	if got.ManagerUserID == nil || *got.ManagerUserID != e.Rep2 {
		t.Errorf("the answer names %v, wanted rep2", got.ManagerUserID)
	}
	if got.RespondedAt == nil {
		t.Error("the answer carries no moment")
	}
}

// A lead may ANSWER a request. They may not settle the commitment, and there is
// no argument by which they could — the state write resolves the plan from the
// caller, so a lead calling it reaches only their own week.
func TestALeadCannotSettleSomebodyElsesCommitment(t *testing.T) {
	e := setupPlan(t)

	commitment := planCommitment(t, e, e.rep1Ctx, "Send the proposal")

	if err := e.store.SetState(e.rep2Ctx, commitment, weeklyplan.StateDone); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a lead settling a rep's commitment got %v, wanted not found", err)
	}
}

// The week's close is idempotent, and that is the whole design: the dispatcher
// ticks more than once inside a week so a worker that was down still backfills.
//
// If a second close re-settled, a commitment the rep completed after the first
// would flip from missed to done and the frozen review would stop matching the
// plan beside it.
func TestClosingTheWeekTwiceAnswersTheSameCounts(t *testing.T) {
	e := setupPlan(t)

	// Last week's plan: two commitments, one done.
	lastWeek := planClock.AddDate(0, 0, -7)
	done := planCommitment(t, e, e.rep1Ctx, "Called the buyer", lastWeek)
	planCommitment(t, e, e.rep1Ctx, "Never got to this", lastWeek)
	if err := e.store.SetState(e.rep1Ctx, done, weeklyplan.StateDone); err != nil {
		t.Fatal(err)
	}

	first, err := e.store.CloseWeek(e.rep1Ctx, planClock)
	if err != nil {
		t.Fatal(err)
	}
	if first.Due != 2 || first.Kept != 1 {
		t.Fatalf("the week closed as %d due / %d kept, wanted 2/1 — without this "+
			"the idempotence check below proves nothing", first.Due, first.Kept)
	}

	// The case idempotence is FOR: a row changing between the two closes.
	//
	// Reaching past the store deliberately — it refuses to settle a closed
	// week, which is the very guard under test, so a legitimate write cannot
	// set this case up. What arrives here is a database in the state a
	// late-landing write or a repair would leave it in, and the question is
	// whether a second close overwrites the frozen answer.
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(context.Background(), `
		UPDATE weekly_plan_commitment SET state = 'done', completed_at = now()
		 WHERE state = 'missed'`); err != nil {
		t.Fatal(err)
	}

	second, err := e.store.CloseWeek(e.rep1Ctx, planClock)
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Errorf("a second close answered %+v, wanted the frozen %+v — a week that "+
			"re-counts is a week whose review stops matching the plan beside it",
			second, first)
	}
}

// A commitment DROPPED counts as neither owed nor kept.
//
// Deciding on Wednesday that a thing is not worth doing is not failing to do
// it, and counting it against a rep teaches them to leave dead commitments open
// rather than say so.
func TestADroppedCommitmentIsNeitherOwedNorKept(t *testing.T) {
	e := setupPlan(t)

	lastWeek := planClock.AddDate(0, 0, -7)
	kept := planCommitment(t, e, e.rep1Ctx, "Did this", lastWeek)
	dropped := planCommitment(t, e, e.rep1Ctx, "Decided against this", lastWeek)
	if err := e.store.SetState(e.rep1Ctx, kept, weeklyplan.StateDone); err != nil {
		t.Fatal(err)
	}
	if err := e.store.SetState(e.rep1Ctx, dropped, weeklyplan.StateDropped); err != nil {
		t.Fatal(err)
	}

	out, err := e.store.CloseWeek(e.rep1Ctx, planClock)
	if err != nil {
		t.Fatal(err)
	}

	if out.Due != 1 || out.Kept != 1 {
		t.Errorf("the week closed as %d due / %d kept, wanted 1/1 — a dropped "+
			"commitment is not work the rep failed to do", out.Due, out.Kept)
	}
}

// A help request lands exactly one audit row and one event, like every other
// mutation in this tree.
func TestAHelpRequestLandsOneAuditRowAndOneEvent(t *testing.T) {
	e := setupPlan(t)
	owner := integration.OwnerConn(t)
	ctx := context.Background()

	commitment := planCommitment(t, e, e.rep1Ctx, "Chase the signature")
	before := countRows(t, owner, ctx, `SELECT count(*) FROM event_outbox
		 WHERE envelope ->> 'type' = 'weekly_plan.help_requested'`)

	if err := e.store.AskForHelp(e.rep1Ctx, commitment, "the buyer has gone quiet"); err != nil {
		t.Fatal(err)
	}

	after := countRows(t, owner, ctx, `SELECT count(*) FROM event_outbox
		 WHERE envelope ->> 'type' = 'weekly_plan.help_requested'`)
	if after != before+1 {
		t.Errorf("the request emitted %d help events, wanted exactly one", after-before)
	}
	audits := countRows(t, owner, ctx, `SELECT count(*) FROM audit_log
		 WHERE entity_type = 'weekly_plan_commitment' AND entity_id = $1 AND action = 'update'`,
		commitment)
	if audits != 1 {
		t.Errorf("the request wrote %d audit rows, wanted exactly one", audits)
	}
}

// Withdrawing a request must not page a lead a second time.
func TestWithdrawingARequestEmitsNoSecondHelpEvent(t *testing.T) {
	e := setupPlan(t)
	owner := integration.OwnerConn(t)
	ctx := context.Background()

	commitment := planCommitment(t, e, e.rep1Ctx, "Chase the signature")
	if err := e.store.AskForHelp(e.rep1Ctx, commitment, "stuck"); err != nil {
		t.Fatal(err)
	}
	before := countRows(t, owner, ctx, `SELECT count(*) FROM event_outbox
		 WHERE envelope ->> 'type' = 'weekly_plan.help_requested'`)

	if err := e.store.AskForHelp(e.rep1Ctx, commitment, ""); err != nil {
		t.Fatal(err)
	}

	after := countRows(t, owner, ctx, `SELECT count(*) FROM event_outbox
		 WHERE envelope ->> 'type' = 'weekly_plan.help_requested'`)
	if after != before {
		t.Errorf("withdrawing emitted %d further help events, wanted none", after-before)
	}
}

// planCommitment writes one commitment and answers its id.
func planCommitment(
	t *testing.T, e *planEnv, ctx context.Context, label string, at ...time.Time,
) ids.UUID {
	t.Helper()
	when := planClock
	if len(at) > 0 {
		when = at[0]
	}
	out, err := e.store.AddCommitment(ctx, when, weeklyplan.NewCommitment{Label: label})
	if err != nil {
		t.Fatalf("writing %q: %v", label, err)
	}
	return out.ID
}

// countRows answers one count under the owner connection.
func countRows(t *testing.T, conn *pgx.Conn, ctx context.Context, query string, args ...any) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(ctx, query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// A typo is correctable, and correcting it keeps what the correction was not
// about.
//
// Before this existed the only way out was to drop the row and write a new one,
// which discards the manager's answer and any help already asked for — the two
// fields on a commitment that were never the rep's to throw away. So the
// assertion is not only that the label moved: it is that the answer survived.
func TestCorrectingACommitmentKeepsWhatItWasNotAbout(t *testing.T) {
	e := setupPlan(t)
	id := planCommitment(t, e, e.rep1Ctx, "Call the Aster buyer on Monday")

	if err := e.store.AskForHelp(e.rep1Ctx, id, "need the Q3 discount sheet"); err != nil {
		t.Fatalf("asking for help: %v", err)
	}
	if err := e.store.Respond(e.rep2Ctx, id, "sent it over"); err != nil {
		t.Fatalf("the lead answering: %v", err)
	}

	label := "Call the Aster buyer on Tuesday"
	if err := e.store.EditCommitment(e.rep1Ctx, id,
		weeklyplan.CommitmentEdit{Label: &label}); err != nil {
		t.Fatalf("correcting the label: %v", err)
	}

	plan, err := e.store.Current(e.rep1Ctx, planClock)
	if err != nil {
		t.Fatalf("reading the plan back: %v", err)
	}
	got := commitmentByID(t, plan, id)
	if got.Label != label {
		t.Errorf("the label reads %q, wanted %q", got.Label, label)
	}
	if got.HelpRequested != "need the Q3 discount sheet" {
		t.Errorf("correcting a typo lost the help request: %q", got.HelpRequested)
	}
	if got.ManagerResponse != "sent it over" {
		t.Errorf("correcting a typo lost the lead's answer: %q", got.ManagerResponse)
	}
}

// Only what is PRESENT changes. A rep fixing a label must not lose the date
// they never mentioned.
func TestCorrectingOneFieldLeavesTheOthersAlone(t *testing.T) {
	e := setupPlan(t)
	due := planClock.AddDate(0, 0, 2)
	written, err := e.store.AddCommitment(e.rep1Ctx, planClock,
		weeklyplan.NewCommitment{Label: "Send the Weber quote", DueOn: &due})
	if err != nil {
		t.Fatalf("writing the commitment: %v", err)
	}

	label := "Send the Weber quote today"
	if err := e.store.EditCommitment(e.rep1Ctx, written.ID,
		weeklyplan.CommitmentEdit{Label: &label}); err != nil {
		t.Fatalf("correcting the label: %v", err)
	}

	plan, err := e.store.Current(e.rep1Ctx, planClock)
	if err != nil {
		t.Fatal(err)
	}
	got := commitmentByID(t, plan, written.ID)
	if got.DueOn == nil {
		t.Fatal("editing the label cleared the due date the request never named")
	}
	if !got.DueOn.Equal(due.Truncate(24*time.Hour)) && got.DueOn.Format(time.DateOnly) != due.Format(time.DateOnly) {
		t.Errorf("the due date moved to %v, wanted %v", got.DueOn, due)
	}
}

// And a date can be CLEARED, which is a different request from not mentioning
// it. Something to do this week, rather than by a day.
func TestADueDateCanBeCleared(t *testing.T) {
	e := setupPlan(t)
	due := planClock.AddDate(0, 0, 2)
	written, err := e.store.AddCommitment(e.rep1Ctx, planClock,
		weeklyplan.NewCommitment{Label: "Draft the Q3 plan", DueOn: &due})
	if err != nil {
		t.Fatalf("writing the commitment: %v", err)
	}

	var cleared *time.Time
	if err := e.store.EditCommitment(e.rep1Ctx, written.ID,
		weeklyplan.CommitmentEdit{DueOn: &cleared}); err != nil {
		t.Fatalf("clearing the date: %v", err)
	}

	plan, err := e.store.Current(e.rep1Ctx, planClock)
	if err != nil {
		t.Fatal(err)
	}
	if got := commitmentByID(t, plan, written.ID); got.DueOn != nil {
		t.Errorf("the due date is still %v after being cleared", got.DueOn)
	}
}

// Somebody else's commitment is NOT FOUND, not refused: whether a colleague
// wrote a particular thing on their week is itself something a stranger may not
// learn.
func TestCorrectingSomebodyElsesCommitmentIsNotFound(t *testing.T) {
	e := setupPlan(t)
	id := planCommitment(t, e, e.rep1Ctx, "rep1's own")

	label := "rewritten by somebody else"
	err := e.store.EditCommitment(e.rep2Ctx, id, weeklyplan.CommitmentEdit{Label: &label})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a teammate editing rep1's commitment got %v, wanted not found", err)
	}
}

// One correction is one audit row and one event, like every other write here.
//
// A no-op correction is NEITHER. Filing an audit row saying a rep changed
// something they did not is a false record, and bumping the version would move
// a row a concurrent reader is holding.
func TestACorrectionLandsOneAuditRowAndANoOpLandsNone(t *testing.T) {
	e := setupPlan(t)
	owner := integration.OwnerConn(t)
	ctx := context.Background()
	id := planCommitment(t, e, e.rep1Ctx, "the original")

	const audits = `SELECT count(*) FROM audit_log
	                 WHERE entity_type = 'weekly_plan_commitment' AND entity_id = $1
	                   AND action = 'update'`
	before := countRows(t, owner, ctx, audits, id)

	label := "the corrected"
	if err := e.store.EditCommitment(e.rep1Ctx, id,
		weeklyplan.CommitmentEdit{Label: &label}); err != nil {
		t.Fatalf("correcting: %v", err)
	}
	if got := countRows(t, owner, ctx, audits, id) - before; got != 1 {
		t.Errorf("one correction filed %d audit rows, wanted 1", got)
	}

	// The same label again changes nothing.
	after := countRows(t, owner, ctx, audits, id)
	if err := e.store.EditCommitment(e.rep1Ctx, id,
		weeklyplan.CommitmentEdit{Label: &label}); err != nil {
		t.Fatalf("re-sending the same label: %v", err)
	}
	if got := countRows(t, owner, ctx, audits, id) - after; got != 0 {
		t.Errorf("a correction that changed nothing filed %d audit rows", got)
	}
}

// commitmentByID finds one commitment on a plan the test just read back.
func commitmentByID(t *testing.T, plan weeklyplan.Plan, id ids.UUID) weeklyplan.Commitment {
	t.Helper()
	for _, c := range plan.Commitments {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("commitment %s is not on the plan", id)
	return weeklyplan.Commitment{}
}

// A closed week is frozen into a review that has already been counted, so its
// commitments stop being editable — the same refusal the settle path makes.
func TestCorrectingACommitmentOnAClosedWeekIsRefused(t *testing.T) {
	e := setupPlan(t)
	id := planCommitment(t, e, e.rep1Ctx, "written while the week was open")

	// CloseWeek settles the week BEFORE the instant it is given — the same
	// window the review covers — so closing the week this commitment lives in
	// means standing a week later.
	if _, err := e.store.CloseWeek(e.rep1Ctx, planClock.AddDate(0, 0, 7)); err != nil {
		t.Fatalf("closing the week: %v", err)
	}

	label := "rewritten after the close"
	err := e.store.EditCommitment(e.rep1Ctx, id, weeklyplan.CommitmentEdit{Label: &label})
	if err == nil {
		t.Fatal("a commitment on a closed week was edited — the review's counts no longer agree with the rows they were counted from")
	}
	var parse *values.ParseError
	if !errors.As(err, &parse) || parse.Code != "week_closed" {
		t.Errorf("editing a closed week gave %v, wanted a week_closed refusal", err)
	}
}
