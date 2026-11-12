// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/licensecheck"
	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

// An unlicensed production installation through the real server: on its
// trial it works and reports the trial; fourteen days after its workspace was
// created it refuses every change, tells every seat on /me, and keeps reads.
func TestAnUnlicensedInstallationLocksWhenItsTrialEnds(t *testing.T) {
	now := time.Now()
	unlicensed := func() licensecheck.Posture {
		return licensecheck.Posture{State: licensecheck.StateAbsent, CheckedAt: now}
	}
	e := apptest.SetupAppWithOptions(t,
		compose.WithLicensePosture(unlicensed),
		func(s *compose.Server, pool *pgxpool.Pool) {
			trial := compose.NewInstallationTrial(pool, runtimeenv.Production, unlicensed, func() time.Time { return now })
			compose.WithInstallationTrial(trial)(s, pool)
		})
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)

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
	if during.Trial == nil || during.Trial.Ended || !during.Trial.EndsAt.Equal(during.Trial.StartedAt.Add(licensecheck.TrialLength)) {
		t.Fatalf("entitlement trial = %+v, want a running 14-day trial", during.Trial)
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
