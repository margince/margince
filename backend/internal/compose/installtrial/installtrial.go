// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package installtrial answers the trial of a production installation that has
// no license, and whether the installation is locked. Its two inputs live in
// different layers: the license posture is platform's, and the start is the
// workspace table's creation time.
package installtrial

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/licensecheck"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

// Trial answers whether this installation runs on its trial, and whether that
// trial has ended. Both serving roles hold one, so the api and the worker lock
// on the same day.
type Trial struct {
	pool    *pgxpool.Pool
	env     runtimeenv.Environment
	posture func() licensecheck.Posture
	now     func() time.Time
	// started caches the start once a workspace exists. It never changes after
	// that, so only an installation not yet set up reads the table again.
	started atomic.Pointer[time.Time]
}

// New reads the license posture at call time, so a key entered while the
// process runs ends the trial without a restart.
func New(pool *pgxpool.Pool, env runtimeenv.Environment, posture func() licensecheck.Posture, now func() time.Time) *Trial {
	return &Trial{pool: pool, env: env, posture: posture, now: now}
}

// Contract is the trial as the entitlement read reports it, or nil when the
// installation runs on none.
func (t *Trial) Contract(ctx context.Context) (*crmcontracts.LicenseTrial, error) {
	trial, ok, err := t.Current(ctx)
	if err != nil || !ok {
		return nil, err
	}
	return &crmcontracts.LicenseTrial{StartedAt: trial.StartedAt, EndsAt: trial.EndsAt(), Ended: trial.Ended(t.now())}, nil
}

// Current answers the trial this installation runs on. ok is false when it runs
// on none. That is a configured license (valid or refused), a development or
// test installation, or one with no workspace yet.
func (t *Trial) Current(ctx context.Context) (licensecheck.Trial, bool, error) {
	if t == nil {
		return licensecheck.Trial{}, false, nil
	}
	return t.currentUnder(ctx, t.posture())
}

// currentUnder is Current judged against one posture snapshot, so a caller that
// already read the posture decides on the same one.
func (t *Trial) currentUnder(ctx context.Context, posture licensecheck.Posture) (licensecheck.Trial, bool, error) {
	if t.env.IsNonProduction() || posture.State != licensecheck.StateAbsent {
		return licensecheck.Trial{}, false, nil
	}
	start, ok, err := t.start(ctx)
	if err != nil || !ok {
		return licensecheck.Trial{}, false, err
	}
	return licensecheck.Trial{StartedAt: start}, true, nil
}

// Ended reports whether this installation's trial is over.
func (t *Trial) Ended(ctx context.Context) (bool, error) {
	trial, ok, err := t.Current(ctx)
	if err != nil || !ok {
		return false, err
	}
	return trial.Ended(t.now()), nil
}

// Lock answers why this installation is locked, and nil when it is not.
// apperrors.ErrLicenseLapsed is a license refused or past its expiry.
// apperrors.ErrTrialEnded is an unlicensed installation whose trial is over.
// Any other error is a failed read.
func (t *Trial) Lock(ctx context.Context) error {
	if t == nil {
		return nil
	}
	posture := t.posture()
	if Lapsed(posture) {
		return apperrors.ErrLicenseLapsed
	}
	trial, ok, err := t.currentUnder(ctx, posture)
	if err != nil {
		return err
	}
	if ok && trial.Ended(t.now()) {
		return apperrors.ErrTrialEnded
	}
	return nil
}

// Lapsed reports a license that no longer admits the installation. There is no
// grace period: a license the module still accepts past its expiry locks too.
func Lapsed(posture licensecheck.Posture) bool {
	return posture.State == licensecheck.StateRejected ||
		(posture.State == licensecheck.StateValid && posture.License.InGrace)
}

// Paused is the question every background lane asks before it works. A failed
// read does not pause: the lane meets the same database failure and reports it.
func (t *Trial) Paused(log *slog.Logger) func(context.Context) bool {
	return func(ctx context.Context) bool {
		err := t.Lock(ctx)
		if errors.Is(err, apperrors.ErrLicenseLapsed) || errors.Is(err, apperrors.ErrTrialEnded) {
			return true
		}
		if err != nil {
			log.WarnContext(ctx, "reading whether the installation is locked failed", "err", err)
		}
		return false
	}
}

// start is the creation time of the installation's first workspace, archived
// ones included. Archiving the workspace and creating another begins no second
// trial.
func (t *Trial) start(ctx context.Context) (time.Time, bool, error) {
	if cached := t.started.Load(); cached != nil {
		return *cached, true, nil
	}
	var first *time.Time
	// pool-exempt: the workspace table is not workspace-scoped; the trial belongs to the installation, before any tenant transaction.
	if err := t.pool.QueryRow(ctx, `SELECT min(created_at) FROM workspace`).Scan(&first); err != nil {
		return time.Time{}, false, fmt.Errorf("installtrial: reading when the trial started: %w", err)
	}
	if first == nil {
		return time.Time{}, false, nil
	}
	t.started.Store(first)
	return *first, true, nil
}
