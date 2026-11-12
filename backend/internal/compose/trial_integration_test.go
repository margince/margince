// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/licensecheck"
	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

func postureOf(state licensecheck.State) func() licensecheck.Posture {
	return func() licensecheck.Posture { return licensecheck.Posture{State: state} }
}

// A production installation with no license runs on a trial that starts when
// its workspace was created, and is locked from the instant it ends.
func TestAnUnlicensedProductionInstallationsTrialStartsWithItsWorkspace(t *testing.T) {
	e := integration.Setup(t)
	ctx := context.Background()
	var created time.Time
	// rls-exempt: test fixture reads the installation's workspace row.
	if err := e.Pool.QueryRow(ctx, `SELECT created_at FROM workspace WHERE id = $1`, e.WS).Scan(&created); err != nil {
		t.Fatal(err)
	}
	now := created
	trial := NewInstallationTrial(e.Pool, runtimeenv.Production, postureOf(licensecheck.StateAbsent), func() time.Time { return now })

	got, ok, err := trial.Current(ctx)
	if err != nil || !ok {
		t.Fatalf("Current = %v, %v, %v; want the trial", got, ok, err)
	}
	if !got.StartedAt.Equal(created) {
		t.Errorf("StartedAt = %v, want the workspace's creation %v", got.StartedAt, created)
	}
	if ended, err := trial.Ended(ctx); err != nil || ended {
		t.Errorf("Ended on the first day = %v, %v; want false", ended, err)
	}
	now = created.Add(licensecheck.TrialLength)
	if ended, err := trial.Ended(ctx); err != nil || !ended {
		t.Errorf("Ended fourteen days later = %v, %v; want true", ended, err)
	}
}

// Only a production installation with no license runs on a trial: a license,
// valid or refused, and a development or test installation never lock.
func TestATrialAppliesOnlyToAnUnlicensedProductionInstallation(t *testing.T) {
	e := integration.Setup(t)
	ctx := context.Background()
	long := func() time.Time { return time.Now().Add(365 * 24 * time.Hour) }
	cases := map[string]*InstallationTrial{
		"licensed":          NewInstallationTrial(e.Pool, runtimeenv.Production, postureOf(licensecheck.StateValid), long),
		"refused license":   NewInstallationTrial(e.Pool, runtimeenv.Production, postureOf(licensecheck.StateRejected), long),
		"development":       NewInstallationTrial(e.Pool, runtimeenv.Development, postureOf(licensecheck.StateAbsent), long),
		"test":              NewInstallationTrial(e.Pool, runtimeenv.Test, postureOf(licensecheck.StateAbsent), long),
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
