// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A Worklist row for a deal that needs its team's manager offers "bring in the
// manager", with the facts that made it qualify. The offer needs a manager on
// the owner's team, and a set-aside lifts only when one of those facts changes.

import (
	"context"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/dealstatus"
	"github.com/margince/margince/backend/internal/compose/managermove"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type managerMoveFixture struct {
	e        *Env
	svc      *dealstatus.Service
	rep      context.Context
	now      time.Time
	pipeline ids.PipelineID
	stage    ids.StageID
}

// repReadingForecast is the rep's grants with the forecast read added or not.
func repReadingForecast(read bool) principal.Permissions {
	perms := AccountRepPerms
	perms.Objects = maps.Clone(AccountRepPerms.Objects)
	perms.Objects["forecast"] = principal.ObjectGrant{Read: read}
	return perms
}

func newManagerMoveFixture(t *testing.T) managerMoveFixture {
	t.Helper()
	e := Setup(t)
	pipeline, stage, _ := DealFixture(t, e)
	now := clocktest.Now(t)
	svc := dealstatus.NewService(e.Pool, e.Deals, e.Activities, dealrooms.NewStore(e.DB()), func() time.Time { return now }).
		WithManagerStandings(managermove.Standings(compose.ForecastStoreForTest(e.Pool)))
	return managerMoveFixture{
		e: e, svc: svc, rep: e.As(e.Rep1, []ids.UUID{e.Team1}, repReadingForecast(true)), now: now, pipeline: pipeline, stage: stage,
	}
}

// touched logs a call on the deal, so it counts as worked with no contact engaged.
func (f managerMoveFixture) touched(t *testing.T, deal ids.UUID) {
	t.Helper()
	at, subject := f.now.Add(-72*time.Hour), "Intro call"
	if _, _, err := f.e.Activities.LogActivity(f.e.Admin(), activities.LogActivityInput{
		Kind: "call", Subject: &subject, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal}},
	}); err != nil {
		t.Fatalf("logging the call: %v", err)
	}
}

// freeze stores one daily team freeze of these open deals, as the daily job does.
// A deal's size and risk signals are then read from it.
func (f managerMoveFixture) freeze(t *testing.T, at time.Time, rows ...forecasting.Contribution) {
	t.Helper()
	f.freezeTeam(t, f.e.Team1, at, rows...)
}

func (f managerMoveFixture) freezeTeam(t *testing.T, team ids.UUID, at time.Time, rows ...forecasting.Contribution) {
	t.Helper()
	period, err := forecasting.ResolvePeriod(forecasting.PeriodQuarter, f.now, 1, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	store := forecasting.NewStore(f.e.DB())
	if err := store.InTx(f.e.Admin(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := store.TakeSnapshot(ctx, tx, forecasting.NewSnapshot{
			Period: period, Scope: forecasting.Scope{Kind: forecasting.ScopeTeam, ID: &team},
			Trigger: forecasting.TriggerDaily, BaseCurrency: "EUR", TakenAt: at,
			Readings: forecasting.Readings{Contributions: rows},
		})
		return err
	}); err != nil {
		t.Fatalf("freezing the team: %v", err)
	}
}

func openRow(deal ids.UUID, base int64, signals ...forecasting.Signal) forecasting.Contribution {
	return forecasting.Contribution{
		DealID: deal.String(), AmountMinor: &base, Currency: "EUR", BaseMinor: &base,
		Source: forecasting.SourceStage, StageProbability: 50, WeightedMinor: base / 2,
		InOpen: true, InEvidence: true, Signals: signals,
	}
}

func (f managerMoveFixture) move(ctx context.Context, t *testing.T, deal ids.UUID) *crmcontracts.DealStatusCardMove {
	t.Helper()
	moves, err := f.svc.CurrentMoves(ctx, []ids.UUID{deal})
	if err != nil {
		t.Fatalf("reading the current moves: %v", err)
	}
	if got, ok := moves[deal]; ok && got.Action == dealstatus.ActionBringInManager {
		return &got
	}
	return nil
}

func TestADealThatNeedsTheManagerOffersTheMoveWithEachReason(t *testing.T) {
	f := newManagerMoveFixture(t)
	f.e.GrantRole(t, f.e.Rep2, "manager")
	owner := f.e.Rep1
	big := f.e.SeedDeal(t, "Big renewal", f.pipeline, f.stage, &owner)
	stuck := f.e.SeedDeal(t, "Stalled expansion", f.pipeline, f.stage, &owner)
	plain := f.e.SeedDeal(t, "Ordinary add-on", f.pipeline, f.stage, &owner)
	single := f.e.SeedDeal(t, "One contact only", f.pipeline, f.stage, &owner)
	f.touched(t, single)
	f.freeze(t, f.now.Add(-time.Hour),
		openRow(big, 900_000), openRow(plain, 100_000), openRow(single, 100_000),
		openRow(stuck, 100_000, forecasting.Signal{Code: "stuck", StageID: f.stage.String()}))

	for name, want := range map[string]struct {
		deal   ids.UUID
		reason string
	}{
		"above the team median": {big, "above the team's median"},
		"one engaged contact":   {single, "only one contact is engaged"},
		"stuck in its stage":    {stuck, "stuck in its stage"},
	} {
		got := f.move(f.rep, t, want.deal)
		if got == nil {
			t.Fatalf("%s: the deal offers no manager move", name)
		}
		if !strings.Contains(got.Reason, want.reason) {
			t.Errorf("%s: reason = %q, want it to name %q", name, got.Reason, want.reason)
		}
		args := *got.Arguments
		if args["assignee_id"] != f.e.Rep2 || args["task_type"] != "review" {
			t.Errorf("%s: arguments = %v, want a review task for the manager", name, args)
		}
	}
	if got := f.move(f.rep, t, plain); got != nil {
		t.Errorf("an ordinary deal offers %+v, want no manager move", got)
	}
}

func TestARepWithNoManagerIsNotOfferedTheMove(t *testing.T) {
	f := newManagerMoveFixture(t)
	owner := f.e.Rep3
	deal := f.e.SeedDeal(t, "Alone on the team", f.pipeline, f.stage, &owner)
	f.touched(t, deal)
	f.e.GrantRole(t, f.e.Rep2, "manager")
	if got := f.move(f.e.As(f.e.Rep3, []ids.UUID{f.e.Team2}, AccountRepPerms), t, deal); got != nil {
		t.Errorf("a rep whose team has no manager is offered %+v, want nothing", got)
	}
	ownDeal := f.e.SeedDeal(t, "The manager's own deal", f.pipeline, f.stage, &f.e.Rep2)
	f.touched(t, ownDeal)
	if got := f.move(f.e.As(f.e.Rep2, []ids.UUID{f.e.Team1}, AccountRepPerms), t, ownDeal); got != nil {
		t.Errorf("a manager is offered themself on their own deal: %+v", got)
	}
}

func TestSettingTheManagerMoveAsideHoldsUntilAFactChanges(t *testing.T) {
	f := newManagerMoveFixture(t)
	f.e.GrantRole(t, f.e.Rep2, "manager")
	owner := f.e.Rep1
	big := f.e.SeedDeal(t, "Big renewal", f.pipeline, f.stage, &owner)
	other := f.e.SeedDeal(t, "Ordinary add-on", f.pipeline, f.stage, &owner)
	f.freeze(t, f.now.Add(-30*time.Hour), openRow(big, 900_000), openRow(other, 100_000))

	offered := f.move(f.rep, t, big)
	if offered == nil || offered.Fingerprint == nil {
		t.Fatalf("move = %+v, want the manager move with a fingerprint to set aside", offered)
	}
	if err := f.svc.DismissMove(f.rep, ids.From[ids.DealKind](big), *offered.Fingerprint); err != nil {
		t.Fatalf("setting the move aside: %v", err)
	}
	if got := f.move(f.rep, t, big); got != nil {
		t.Fatalf("the set-aside move is back with no change: %+v", got)
	}

	f.freeze(t, f.now.Add(-time.Hour), openRow(big, 950_000), openRow(other, 100_000))
	if got := f.move(f.rep, t, big); got == nil {
		t.Error("the deal's amount changed and the move is still set aside")
	}
}

func TestForecastReasonsFollowTheReadersForecastAccess(t *testing.T) {
	f := newManagerMoveFixture(t)
	f.e.GrantRole(t, f.e.Rep2, "manager")
	owner, other := f.e.Rep1, f.e.Rep3
	stuck := f.e.SeedDeal(t, "Stalled expansion", f.pipeline, f.stage, &owner)
	foreign := f.e.SeedDeal(t, "Other team's stalled deal", f.pipeline, f.stage, &other)
	f.freeze(t, f.now.Add(-time.Hour),
		openRow(stuck, 100_000, forecasting.Signal{Code: "stuck", StageID: f.stage.String()}))
	f.freezeTeam(t, f.e.Team2, f.now.Add(-time.Hour),
		openRow(foreign, 100_000, forecasting.Signal{Code: "stuck", StageID: f.stage.String()}))
	store := compose.ForecastStoreForTest(f.e.Pool)
	standings := func(ctx context.Context) map[ids.UUID]forecasting.DealStanding {
		t.Helper()
		var got map[ids.UUID]forecasting.DealStanding
		err := database.WithWorkspaceTx(ctx, f.e.Pool, func(tx pgx.Tx) error {
			var err error
			got, err = store.DealStandings(ctx, tx, []ids.UUID{stuck, foreign})
			return err
		})
		if err != nil {
			t.Fatalf("reading the standings: %v", err)
		}
		return got
	}

	if got := standings(f.rep); !got[stuck].Stuck {
		t.Fatalf("a rep reading the forecast sees %+v for their own stuck deal, want stuck", got[stuck])
	}
	if got := standings(f.rep); got[foreign].Stuck {
		t.Errorf("a rep sees the stuck flag of a deal outside their teams: %+v", got[foreign])
	}
	blind := f.e.As(f.e.Rep1, []ids.UUID{f.e.Team1}, repReadingForecast(false))
	if got := standings(blind); len(got) != 0 {
		t.Errorf("a reader without forecast read gets %+v, want no standings and no error", got)
	}
	if got := f.move(blind, t, stuck); got != nil {
		t.Errorf("a reader without forecast read is offered %+v for a deal only the forecast calls stuck", got)
	}
}

func TestDealActionsBetweenNamesTasksDoneAndMeetingsHeldInsideTheWindow(t *testing.T) {
	f := newManagerMoveFixture(t)
	owner := f.e.Rep1
	deal := f.e.SeedDeal(t, "Worked deal", f.pipeline, f.stage, &owner)
	link := []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal}}
	subject, held := "Send the contract", "held"
	task, _, err := f.e.Activities.LogActivity(f.e.Admin(), activities.LogActivityInput{Kind: "task", Subject: &subject, Source: "manual", Links: link})
	if err != nil {
		t.Fatalf("logging the task: %v", err)
	}
	if _, err := f.e.Activities.CompleteTask(f.e.Admin(), ids.From[ids.ActivityKind](ids.UUID(task.Id)), f.now.Add(48*time.Hour)); err != nil {
		t.Fatalf("completing the task: %v", err)
	}
	meetingAt, meeting := f.now.Add(-2*time.Hour), "Review meeting"
	if _, _, err := f.e.Activities.LogActivity(f.e.Admin(), activities.LogActivityInput{
		Kind: "meeting", Subject: &meeting, MeetingStatus: &held, OccurredAt: &meetingAt, Source: "manual", Links: link,
	}); err != nil {
		t.Fatalf("logging the meeting: %v", err)
	}
	read := func(after, until time.Time) []activities.DealAction {
		t.Helper()
		var got []activities.DealAction
		err := database.WithWorkspaceTx(f.e.Admin(), f.e.Pool, func(tx pgx.Tx) error {
			var err error
			got, err = activities.DealActionsBetweenTx(f.e.Admin(), tx, []ids.UUID{deal}, after, until)
			return err
		})
		if err != nil {
			t.Fatalf("reading the actions: %v", err)
		}
		return got
	}

	got := read(f.now.Add(-24*time.Hour), f.now.Add(24*time.Hour))
	if len(got) != 2 || got[0].Kind != activities.DealActionMeetingHeld || got[1].Kind != activities.DealActionTaskDone {
		t.Fatalf("actions = %+v, want the meeting held then the task done, oldest first", got)
	}
	if got := read(f.now.Add(12*time.Hour), f.now.Add(24*time.Hour)); len(got) != 0 {
		t.Errorf("a window after both actions lists %+v, want none", got)
	}
}
