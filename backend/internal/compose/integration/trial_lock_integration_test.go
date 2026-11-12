// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/licensecheck"
	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

func postureOf(state licensecheck.State) func() licensecheck.Posture {
	return func() licensecheck.Posture { return licensecheck.Posture{State: state} }
}

// workspaceCreated is the instant the trial counts from, read rather than
// assumed. Every clock below is dated from the row, none from the wall.
func workspaceCreated(t *testing.T, pool *pgxpool.Pool) time.Time {
	t.Helper()
	var created time.Time
	// rls-exempt: test fixture reads the installation's workspace row.
	if err := pool.QueryRow(context.Background(), `SELECT min(created_at) FROM workspace`).Scan(&created); err != nil {
		t.Fatal(err)
	}
	return created
}

// Only a production installation with no license runs on a trial: a license,
// valid or refused, and a development or test installation never lock.
func TestATrialAppliesOnlyToAnUnlicensedProductionInstallation(t *testing.T) {
	e := Setup(t)
	ctx := context.Background()
	late := workspaceCreated(t, e.Pool).Add(365 * 24 * time.Hour)
	at := func() time.Time { return late }
	cases := map[string]*compose.InstallationTrial{
		"licensed":          compose.NewInstallationTrial(e.Pool, runtimeenv.Production, postureOf(licensecheck.StateValid), at),
		"refused license":   compose.NewInstallationTrial(e.Pool, runtimeenv.Production, postureOf(licensecheck.StateRejected), at),
		"development":       compose.NewInstallationTrial(e.Pool, runtimeenv.Development, postureOf(licensecheck.StateAbsent), at),
		"test":              compose.NewInstallationTrial(e.Pool, runtimeenv.Test, postureOf(licensecheck.StateAbsent), at),
		"no trial wired in": nil,
	}
	for name, trial := range cases {
		if _, ok, err := trial.Current(ctx); err != nil || ok {
			t.Errorf("%s: Current ok = %v, err = %v; want no trial", name, ok, err)
		}
		if ended, err := trial.Ended(ctx); err != nil || ended {
			t.Errorf("%s: Ended = %v, err = %v; want false", name, ended, err)
		}
	}
}

// An unlicensed production installation, through the real server. Its trial
// starts with its workspace, and it works and reports the trial while it runs.
// From the instant the trial ends it refuses every change and tells every seat
// on /me, and reads still answer.
func TestAnUnlicensedInstallationLocksWhenItsTrialEnds(t *testing.T) {
	var now time.Time
	unlicensed := postureOf(licensecheck.StateAbsent)
	e := apptest.SetupAppWithOptions(t,
		compose.WithLicensePosture(unlicensed),
		func(s *compose.Server, pool *pgxpool.Pool) {
			trial := compose.NewInstallationTrial(pool, runtimeenv.Production, unlicensed, func() time.Time { return now })
			compose.WithInstallationTrial(trial)(s, pool)
		})
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	now = workspaceCreated(t, e.Pool)

	var during struct {
		Trial *struct {
			StartedAt time.Time `json:"started_at"`
			EndsAt    time.Time `json:"ends_at"`
			Ended     bool      `json:"ended"`
		} `json:"trial"`
	}
	if status := e.Call(t, "GET", "/v1/installation/license", nil, nil, &during); status != http.StatusOK {
		t.Fatalf("read the entitlement on the trial → %d", status)
	}
	if during.Trial == nil || during.Trial.Ended || !during.Trial.StartedAt.Equal(now) ||
		!during.Trial.EndsAt.Equal(now.Add(licensecheck.TrialLength)) {
		t.Fatalf("entitlement trial = %+v, want a running 14-day trial from the workspace's creation %v", during.Trial, now)
	}
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{"source": "manual", "full_name": "On The Trial"}, nil, nil); status != http.StatusCreated {
		t.Fatalf("create a contact on the trial → %d, want 201", status)
	}

	now = during.Trial.EndsAt
	var refusal struct {
		Code string `json:"code"`
	}
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{"source": "manual", "full_name": "After The Trial"}, nil, &refusal); status != http.StatusForbidden || refusal.Code != "trial_ended" {
		t.Fatalf("create a contact after the trial → %d %q, want 403 trial_ended", status, refusal.Code)
	}
	var me struct {
		TrialEnded *bool `json:"trial_ended"`
	}
	if status := e.Call(t, "GET", "/v1/me", nil, nil, &me); status != http.StatusOK || me.TrialEnded == nil || !*me.TrialEnded {
		t.Fatalf("/me after the trial → %d trial_ended=%v, want 200 and true", status, me.TrialEnded)
	}
	if status := e.Call(t, "GET", "/v1/contacts", nil, nil, nil); status != http.StatusOK {
		t.Errorf("list contacts after the trial → %d, want 200: the data stays readable", status)
	}
}
