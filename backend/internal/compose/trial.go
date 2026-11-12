// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The trial of a production installation that has no license. Composed here
// because its two inputs live in different layers: the license posture is
// platform's, and the start is the workspace table's creation time.

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/licensecheck"
	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

// InstallationTrial answers whether this installation runs on its trial, and
// whether that trial has ended. Both serving roles hold one, so the api and the
// worker lock on the same day.
type InstallationTrial struct {
	pool    *pgxpool.Pool
	env     runtimeenv.Environment
	posture func() licensecheck.Posture
	now     func() time.Time
	// started caches the start once a workspace exists. It never changes after
	// that, so only an installation not yet set up reads the table again.
	started atomic.Pointer[time.Time]
}

// NewInstallationTrial reads the license posture at call time, so a key entered
// while the process runs ends the trial without a restart.
func NewInstallationTrial(pool *pgxpool.Pool, env runtimeenv.Environment, posture func() licensecheck.Posture, now func() time.Time) *InstallationTrial {
	return &InstallationTrial{pool: pool, env: env, posture: posture, now: now}
}

// Current answers the trial this installation runs on. ok is false when it runs
// on none: a license is configured (valid or refused), the installation is a
// development or test one, or no workspace has been set up yet.
func (t *InstallationTrial) Current(ctx context.Context) (licensecheck.Trial, bool, error) {
	if t == nil || t.env.IsNonProduction() || t.posture().State != licensecheck.StateAbsent {
		return licensecheck.Trial{}, false, nil
	}
	start, ok, err := t.start(ctx)
	if err != nil || !ok {
		return licensecheck.Trial{}, false, err
	}
	return licensecheck.Trial{StartedAt: start}, true, nil
}

// Ended reports whether this installation's trial is over, which is the one
// condition that locks it.
func (t *InstallationTrial) Ended(ctx context.Context) (bool, error) {
	trial, ok, err := t.Current(ctx)
	if err != nil || !ok {
		return false, err
	}
	return trial.Ended(t.now()), nil
}

// start is the creation time of the installation's first workspace, archived
// ones included, so archiving the workspace and creating another does not
// begin a second trial.
func (t *InstallationTrial) start(ctx context.Context) (time.Time, bool, error) {
	if cached := t.started.Load(); cached != nil {
		return *cached, true, nil
	}
	var first *time.Time
	// rls-exempt: the workspace table is not workspace-scoped; the trial belongs to the installation, before any tenant transaction.
	if err := t.pool.QueryRow(ctx, `SELECT min(created_at) FROM workspace`).Scan(&first); err != nil {
		return time.Time{}, false, fmt.Errorf("compose: reading when the trial started: %w", err)
	}
	if first == nil {
		return time.Time{}, false, nil
	}
	t.started.Store(first)
	return *first, true, nil
}
