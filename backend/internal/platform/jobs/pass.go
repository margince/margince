// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

// When a periodic kind next runs, for the surfaces that make somebody wait.
//
// A screen that says "waiting" and nothing else leaves a contact two readings,
// broken and slow, and both are wrong when the pipeline is working exactly as
// declared on a clock the screen never mentions. The clock is knowable here and
// nowhere else: the cadence is the compiled declaration, and whether a pass is
// moving is the job table. It lives beside Stats and WorkspaceHealth for the
// reason that file gives — every statement over river_job is a hand-imposed
// scope, and two readers spelling it in two packages drift into two answers
// about one table.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pass is one periodic kind's schedule as a caller can state it to a contact.
type Pass struct {
	// Every is the declared cadence. Zero for a kind no clock runs, which is a
	// different sentence from one whose next run is merely unknown.
	Every time.Duration
	// Running says a pass of this kind is in flight right now.
	Running bool
	// Queued says a run is DUE and no worker has picked it up. It is its own
	// answer rather than a time: a due row's scheduled_at is already in the
	// past, so reporting it as the next pass would print a moment that has been
	// and gone. It is also the state that distinguishes a slow installation from
	// a stopped one — work sitting queued is a worker that is not running, which
	// is the question somebody watching an unmoving counter is actually asking.
	Queued bool
	// NextAt is when the next pass runs, nil when this deployment cannot say.
	// Nil is not "soon": it is what a fleet whose last pass has already aged out
	// of River's retention honestly knows, and a caller must say the cadence
	// instead rather than invent a time.
	NextAt *time.Time
}

// PassFor answers where one periodic kind stands.
//
// The next time is taken from a row River has ALREADY scheduled where there is
// one, because that is the fire time rather than a projection of it. Failing
// that it is the last completed run's OWN moment plus the cadence, which is the
// number River's interval produces: the ticks are an interval apart from each
// other, not from when the work happened to finish. Projecting from finalized_at
// instead would run late by however long the pass took — up to the twenty
// minutes these two are allowed, on the screen that exists to say when. A kind with neither — nothing scheduled, nothing completed inside
// River's retention — answers nil, and the caller says how often instead of
// when.
//
// A row that is DUE is neither of those and gets its own answer. `retryable` is
// counted with the runnable states rather than left out: a pass that failed and
// is waiting to be tried again is still a pass that is coming, and reporting the
// cadence past it would tell somebody to expect a run at a time when what is
// really pending is a retry.
//
// It is deliberately NOT workspace-scoped. A periodic verdict pass is one tick
// for the installation, and River carries no workspace on the row that schedules
// it; a scope clause here would answer "no pass is coming" for every workspace
// on a fleet where one is.
func PassFor(ctx context.Context, pool *pgxpool.Pool, kind string) (Pass, error) {
	out := Pass{}
	if spec, ok := SpecFor(kind); ok {
		out.Every = spec.Cadence.Fixed
	}
	// The three questions in one round trip, and the DUE one is asked apart from
	// the future one on purpose. A row River has made `available` carries a
	// scheduled_at in the past — it is runnable now — so folding the two into one
	// "next run" would answer with a moment that has already gone.
	const q = `
		SELECT EXISTS (SELECT 1 FROM river_job WHERE kind = $1 AND state::text = 'running'),
		       EXISTS (SELECT 1 FROM river_job
		                WHERE kind = $1 AND state::text IN ('available','scheduled','retryable')
		                  AND scheduled_at <= now()),
		       (SELECT min(scheduled_at) FROM river_job
		         WHERE kind = $1 AND state::text IN ('available','scheduled','retryable')
		           AND scheduled_at > now()),
		       (SELECT scheduled_at FROM river_job
		         WHERE kind = $1 AND state::text = 'completed'
		         ORDER BY scheduled_at DESC LIMIT 1)`
	var scheduled, ranAt *time.Time
	if err := pool.QueryRow(ctx, q, kind).Scan(&out.Running, &out.Queued, &scheduled, &ranAt); err != nil {
		return Pass{}, fmt.Errorf("jobs: reading when %q next runs: %w", kind, err)
	}
	out.NextAt = nextPass(scheduled, ranAt, out.Every)
	return out, nil
}

// nextPass answers when the next tick lands: the row River has already
// scheduled where there is one, else the last completed run's OWN moment plus
// the cadence, else nothing.
//
// A pure function because it is the whole decision, and the decision is the
// part that can be wrong in a way nobody sees — a screen that exists to say
// when will print whatever this returns, and a plausible-looking time is
// indistinguishable from a right one.
//
// Projecting from the completed run's scheduled_at rather than from when it
// FINISHED: River's ticks are an interval apart from each other, not from the
// end of the work, so projecting off the finish runs late by however long the
// pass took.
func nextPass(scheduled, ranAt *time.Time, every time.Duration) *time.Time {
	if scheduled != nil {
		return scheduled
	}
	if ranAt == nil || every <= 0 {
		return nil
	}
	next := ranAt.Add(every)
	return &next
}
