// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package verdictclock reads when one periodic verdict pass next runs.
//
// The two halves belong to different owners and neither may reach the other.
// The schedule is the job queue's, and capture must not learn to read river_job.
// Kinds come from the args types the runner registers, not from string
// literals, so a renamed kind cannot leave this reading a queue nobody writes.
package verdictclock

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/jobs"
)

// Setting reads the interval an installation setting holds for a cadence key.
type Setting func(ctx context.Context, key string) (time.Duration, error)

// Pass answers for one pass kind. A kind whose cadence is an installation
// setting declares no fixed interval. Its interval is read from that setting,
// the same value the worker schedules from.
func Pass(pool *pgxpool.Pool, kind string, setting Setting) capture.VerdictPass {
	return func(ctx context.Context) (capture.VerdictClock, error) {
		pass, err := jobs.PassFor(ctx, pool, kind)
		if err != nil {
			return capture.VerdictClock{}, err
		}
		if spec, ok := jobs.SpecFor(kind); ok && spec.Cadence.Setting != "" {
			if pass.Every, err = setting(ctx, spec.Cadence.Setting); err != nil {
				return capture.VerdictClock{}, err
			}
		}
		return capture.VerdictClock{
			Every: pass.Every, Running: pass.Running, Queued: pass.Queued, NextAt: pass.NextAt,
		}, nil
	}
}
