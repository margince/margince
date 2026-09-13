// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/weeklyplan"
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
	w := teamSnapshotWorker(e)
	probe := &weeklyMeasurementProbe{t: t, env: e}
	w.narrator = probe
	if err := w.measureWorkspace(e.Admin(), e.WS, teamJobClock); err != nil {
		t.Fatal(err)
	}
	if probe.calls == 0 {
		t.Fatal("model boundary was never exercised")
	}
}

func TestWeeklyReviewHourAppliesToTeamsAndSundayCanCatchUp(t *testing.T) {
	for _, tc := range []struct {
		name  string
		at    time.Time
		ready bool
	}{
		{"before Monday opening", time.Date(2026, 6, 8, 5, 59, 0, 0, time.UTC), false},
		{"Monday opening", time.Date(2026, 6, 8, 6, 0, 0, 0, time.UTC), true},
		{"Sunday catchup", time.Date(2026, 6, 14, 1, 0, 0, 0, time.UTC), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			e.WsExec(t, `UPDATE setting SET value='"UTC"'::jsonb WHERE key='installation.timezone'`)
			seedManagerRoles(t, e, e.Rep1, e.Rep2, e.Rep3)
			w := teamSnapshotWorker(e)
			if err := w.measureWorkspace(e.Admin(), e.WS, tc.at); err != nil {
				t.Fatal(err)
			}
			got := e.WsCount(t, `SELECT count(*) FROM team_weekly_review WHERE reps_counted>0`)
			if tc.ready && got != 2 {
				t.Fatalf("expected both team reviews, got %d", got)
			}
			if !tc.ready && e.WsCount(t, `SELECT count(*) FROM team_weekly_review`) > 0 {
				t.Fatal("premature team review was frozen")
			}
		})
	}
}

func TestScheduledMeasurementIncludesTheRealPlanOutcome(t *testing.T) {
	e := integration.Setup(t)
	seedManagerRoles(t, e, e.Rep1, e.Rep2, e.Rep3)
	e.WsExec(t, `UPDATE role SET permissions=jsonb_set(permissions,'{objects,forecast}','{"read":true,"create":true}'::jsonb) WHERE key='team_lead_under_test'`)
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
