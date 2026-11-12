// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/installtrial"
	"github.com/margince/margince/backend/internal/platform/licensecheck"
	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

// lockedRecheck is how often a locked worker reads the license again. The api
// re-checks the moment an admin enters a key; this role learns of it here.
const lockedRecheck = 5 * time.Minute

type workPauseKey struct{}

// pauseAfterTrial binds the question every lane and job asks before it works:
// has this unlicensed installation's trial ended? It rides the context because
// every lane is started on it, and a parameter would thread through each one.
func pauseAfterTrial(ctx context.Context, pool *pgxpool.Pool, license *licensecheck.Watcher, env runtimeenv.Environment, log *slog.Logger) context.Context {
	trial := installtrial.New(pool, env, license.Posture, time.Now)
	paused := func(ctx context.Context) bool {
		ended, err := trial.Ended(ctx)
		if err != nil {
			// Not paused: the lane or job that runs meets the same database
			// failure and reports it as its own.
			log.WarnContext(ctx, "reading whether the trial has ended failed", "err", err)
			return false
		}
		return ended
	}
	go recheckWhileLocked(ctx, license, paused)
	return context.WithValue(ctx, workPauseKey{}, paused)
}

// workPause is the pause pauseAfterTrial bound, or nil for a context without
// one, which never pauses.
func workPause(ctx context.Context) func(context.Context) bool {
	paused, _ := ctx.Value(workPauseKey{}).(func(context.Context) bool)
	return paused
}

// recheckWhileLocked re-reads the license while the worker is paused, so a key
// sealed through the api resumes this role without a restart.
func recheckWhileLocked(ctx context.Context, license *licensecheck.Watcher, paused func(context.Context) bool) {
	ticker := time.NewTicker(lockedRecheck)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if paused(ctx) {
				//craft:ignore swallowed-errors Recheck logs its own fault and keeps the last posture; the loop has nobody to tell
				_ = license.Recheck(ctx)
			}
		}
	}
}
