// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/weeklyplan"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

type weeklyMeasurementProbe struct {
	t     *testing.T
	env   *integration.Env
	calls int
}

func (p *weeklyMeasurementProbe) Complete(context.Context, model.Request) (model.Response, error) {
	p.calls++
	if got := p.env.WsCount(p.t, `SELECT count(*) FROM weekly_review`); got < 3 {
		p.t.Errorf("model started with only %d personal reviews", got)
	}
	if got := p.env.WsCount(p.t, `SELECT count(*) FROM team_weekly_review WHERE reps_counted>0 AND reps_unread=0`); got != 2 {
		p.t.Errorf("model started before both teams were measured: %d", got)
	}
	return model.Response{}, context.Canceled
}

func TestWeeklyMeasurementsFinishBeforeAnyModelCall(t *testing.T) {
	e := integration.Setup(t)
	seedManagerRoles(t, e, e.Rep1, e.Rep2, e.Rep3)
	seedMeasuredNarrativeSources(t, e)
	w := teamSnapshotWorker(e)
	probe := &weeklyMeasurementProbe{t: t, env: e}
	w.narrator = probe
	if err := w.measureWorkspace(e.Admin(), e.WS, teamJobClock); err != nil {
		t.Fatal(err)
	}
	if probe.calls == 0 {
		review, err := w.engine.LatestReview(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms), nil)
		if err != nil {
			t.Fatal(err)
		}
		facts, err := json.Marshal(review.NumericSummary)
		if err != nil {
			t.Fatal(err)
		}
		t.Fatalf("model boundary was never exercised; measured sources: %s", facts)
	}
}

func TestTeamReviewsKeepTheClosedPeriodAtAnyDeliveryHour(t *testing.T) {
	for _, test := range []struct {
		name string
		at   time.Time
		week string
	}{
		{"before the week closes", time.Date(2026, 6, 7, 23, 59, 0, 0, time.UTC), "2026-05-25"},
		{"Monday opening", time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC), "2026-06-01"},
		{"before the old delivery floor", time.Date(2026, 6, 8, 5, 59, 0, 0, time.UTC), "2026-06-01"},
		{"Sunday catchup", time.Date(2026, 6, 14, 1, 0, 0, 0, time.UTC), "2026-06-01"},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := integration.Setup(t)
			seedManagerRoles(t, e, e.Rep1, e.Rep2, e.Rep3)
			if err := teamSnapshotWorker(e).measureWorkspace(e.Admin(), e.WS, test.at); err != nil {
				t.Fatal(err)
			}
			var count int
			var binds reportingBindings
			if err := integration.OwnerConn(t).QueryRow(e.Admin(), `SELECT count(*) FROM team_weekly_review WHERE reps_counted>0 AND local_week_start=`+binds.add(test.week)+`::date`, binds.values...).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 2 {
				t.Fatalf("%d teams use closed period %s", count, test.week)
			}
		})
	}
}

func TestScheduledMeasurementIncludesTheRealPlanOutcome(t *testing.T) {
	e := integration.Setup(t)
	seedManagerRoles(t, e, e.Rep1, e.Rep2, e.Rep3)
	e.WsExec(t, `UPDATE role SET permissions=jsonb_set(permissions,'{objects,forecast}','{"read":true,"create":true}'::jsonb) WHERE key='manager'`)
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)
	plan := weeklyPlanStore(e.Pool)
	during := teamJobClock.AddDate(0, 0, -7)
	if _, err := plan.StartWeek(ctx, during); err != nil {
		t.Fatal(err)
	}
	if _, err := plan.AddCommitment(ctx, during, weeklyplan.NewCommitment{Label: "Send the proposal"}); err != nil {
		t.Fatal(err)
	}
	w := teamSnapshotWorker(e)
	if err := w.measureWorkspace(e.Admin(), e.WS, teamJobClock); err != nil {
		t.Fatal(err)
	}
	review, err := w.engine.LatestReview(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if review.Counts.CommitmentsDue != 1 || review.Counts.CommitmentsKept != 0 {
		t.Fatalf("plan outcome missing: %+v", review.Counts)
	}
	if len(review.Outlook) != 3 {
		t.Fatalf("scheduled forecast horizons missing: %d", len(review.Outlook))
	}
}

func TestReadOnlyPlanAuthorityStillReceivesRecordedWork(t *testing.T) {
	e := integration.Setup(t)
	seedManagerRoles(t, e, e.Rep1, e.Rep2, e.Rep3)
	e.WsExec(t, `UPDATE role SET permissions=jsonb_set(permissions,'{objects,weekly_plan}','{"read":true,"update":false}'::jsonb) WHERE key='manager'`)
	w := teamSnapshotWorker(e)
	if err := w.measureWorkspace(e.Admin(), e.WS, teamJobClock); err != nil {
		t.Fatal(err)
	}
	review, err := w.engine.LatestReview(e.As(e.Rep1, nil, integration.AdminPerms), nil)
	if err != nil {
		t.Fatalf("read-only member lost their recorded-work review: %v", err)
	}
	if review.ID.IsZero() || review.Counts.CommitmentsDue != 0 {
		t.Fatalf("unexpected read-only review: %+v", review)
	}
	if e.WsCount(t, `SELECT count(*) FROM weekly_plan`) != 0 {
		t.Fatal("read-only report created a plan")
	}
}

func seedMeasuredNarrativeSources(t *testing.T, e *integration.Env) {
	t.Helper()
	at := time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	owner := ids.From[ids.UserKind](e.Rep1)
	pipeline, stage, won := integration.DealFixture(t, e)
	deal, err := e.Deals.WithClock(func() time.Time { return at }).CreateDeal(ctx, deals.CreateDealInput{Name: "Measured opportunity", PipelineID: pipeline, StageID: stage, OwnerID: &owner, AmountMinor: new(int64(10000)), Currency: new("EUR"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	closedAt := time.Date(2026, 6, 3, 9, 0, 0, 0, time.UTC)
	if _, err := e.Deals.WithClock(func() time.Time { return closedAt }).AdvanceDeal(ctx, ids.From[ids.DealKind](ids.UUID(deal.Id)), deals.AdvanceDealInput{ToStageID: won, WonWithoutContractReason: new("purchase_order")}); err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `UPDATE role SET permissions=jsonb_set(permissions,'{objects,lead}','{"read":true}'::jsonb) WHERE key='manager'`)
	if _, _, err := e.Contacts.WithClock(func() time.Time { return at }).CreateLead(ctx, contacts.CreateLeadInput{FullName: new("Measured lead"), OwnerID: &owner, Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.Activities.WithClock(func() time.Time { return at }).LogActivity(ctx, activities.LogActivityInput{Kind: "task", Subject: new("Send the quote"), AssigneeID: &owner, Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	// Only fixture timestamps move; each source was written through its real writer.
	var binds reportingBindings
	stamp, who := binds.add(at), binds.add(e.Rep1)
	e.WsExec(t, `UPDATE deal SET created_at=`+stamp+` WHERE owner_id=`+who, binds.values...)
	e.WsExec(t, `UPDATE lead SET created_at=`+stamp+` WHERE owner_id=`+who, binds.values...)
	e.WsExec(t, `UPDATE activity SET created_at=`+stamp+` WHERE kind='task' AND assignee_id=`+who, binds.values...)
	logHeldMeeting(t, e, e.Rep1, at)
	plan := weeklyPlanStore(e.Pool)
	for _, week := range []time.Time{at, teamJobClock.AddDate(0, 0, -7)} {
		if _, err := plan.StartWeek(ctx, week); err != nil {
			t.Fatal(err)
		}
	}
}
