// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A Worklist row for a deal that needs its team's manager offers "bring in the
// manager", with the facts that made it qualify. The offer needs a manager on
// the owner's team, and a set-aside lifts only when one of those facts changes.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/dealstatus"
	"github.com/margince/margince/backend/internal/compose/managermove"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type managerMoveFixture struct {
	e        *Env
	svc      *dealstatus.Service
	rep      context.Context
	now      time.Time
	pipeline ids.PipelineID
	stage    ids.StageID
}

func newManagerMoveFixture(t *testing.T) managerMoveFixture {
	t.Helper()
	e := Setup(t)
	pipeline, stage, _ := DealFixture(t, e)
	now := clocktest.Now(t)
	svc := dealstatus.NewService(e.Pool, e.Deals, e.Activities, dealrooms.NewStore(e.DB()), func() time.Time { return now }).
		WithManagerStandings(managermove.Standings)
	return managerMoveFixture{
		e: e, svc: svc, rep: e.As(e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms), now: now, pipeline: pipeline, stage: stage,
	}
}

// touched logs a call on the deal, so it counts as worked with no contact engaged.
func (f managerMoveFixture) touched(t *testing.T, deal ids.UUID) {
	t.Helper()
	at := f.now.Add(-72 * time.Hour)
	if _, _, err := f.e.Activities.LogActivity(f.e.Admin(), activities.LogActivityInput{
		Kind: "call", OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal}},
	}); err != nil {
		t.Fatalf("logging the call: %v", err)
	}
}

// freeze stores one daily team freeze holding these open deals, the way the
// daily job does, so a deal's size and risk signals are read from it.
func (f managerMoveFixture) freeze(t *testing.T, at time.Time, rows ...forecasting.Contribution) {
	t.Helper()
	period, err := forecasting.ResolvePeriod(forecasting.PeriodQuarter, f.now, 1, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	store := forecasting.NewStore(f.e.DB())
	team := f.e.Team1
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

func (f managerMoveFixture) move(t *testing.T, ctx context.Context, deal ids.UUID) *crmcontracts.DealStatusCardMove {
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
		got := f.move(t, f.rep, want.deal)
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
	if got := f.move(t, f.rep, plain); got != nil {
		t.Errorf("an ordinary deal offers %+v, want no manager move", got)
	}
}

func TestARepWithNoManagerIsNotOfferedTheMove(t *testing.T) {
	f := newManagerMoveFixture(t)
	owner := f.e.Rep3
	deal := f.e.SeedDeal(t, "Alone on the team", f.pipeline, f.stage, &owner)
	f.touched(t, deal)
	f.e.GrantRole(t, f.e.Rep2, "manager")
	if got := f.move(t, f.e.As(f.e.Rep3, []ids.UUID{f.e.Team2}, AccountRepPerms), deal); got != nil {
		t.Errorf("a rep whose team has no manager is offered %+v, want nothing", got)
	}
	ownDeal := f.e.SeedDeal(t, "The manager's own deal", f.pipeline, f.stage, &f.e.Rep2)
	f.touched(t, ownDeal)
	if got := f.move(t, f.e.As(f.e.Rep2, []ids.UUID{f.e.Team1}, AccountRepPerms), ownDeal); got != nil {
		t.Errorf("a manager is offered themself on their own deal: %+v", got)
	}
}

func TestSettingTheManagerMoveAsideHoldsUntilAFactChanges(t *testing.T) {
	f := newManagerMoveFixture(t)
	f.e.GrantRole(t, f.e.Rep2, "manager")
	owner := f.e.Rep1
	big := f.e.SeedDeal(t, "Big renewal", f.pipeline, f.stage, &owner)
	other := f.e.SeedDeal(t, "Ordinary add-on", f.pipeline, f.stage, &owner)
	f.freeze(t, f.now.Add(-2*time.Hour), openRow(big, 900_000), openRow(other, 100_000))

	offered := f.move(t, f.rep, big)
	if offered == nil || offered.Fingerprint == nil {
		t.Fatalf("move = %+v, want the manager move with a fingerprint to set aside", offered)
	}
	if err := f.svc.DismissMove(f.rep, ids.From[ids.DealKind](big), *offered.Fingerprint); err != nil {
		t.Fatalf("setting the move aside: %v", err)
	}
	if got := f.move(t, f.rep, big); got != nil {
		t.Fatalf("the set-aside move is back with no change: %+v", got)
	}

	f.freeze(t, f.now.Add(-time.Hour), openRow(big, 950_000), openRow(other, 100_000))
	if got := f.move(t, f.rep, big); got == nil {
		t.Error("the deal's amount changed and the move is still set aside")
	}
}
