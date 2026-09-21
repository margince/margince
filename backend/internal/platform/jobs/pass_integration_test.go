// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobs_test

// Real-Postgres proof of the read behind "when does this pass run next".
//
// A screen that tells somebody to wait is only worth the number it prints, and
// the number comes from three sources in order of authority: a run River has
// already scheduled, the last completed run plus the declared cadence, and
// nothing at all. The SQL is the whole of that ordering, so a fake pool would
// prove the fixture rather than the query.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/jobs"
)

// aScheduledKind is a kind api/jobs.yaml declares with a fixed, hourly
// cadence, so the spec lookup answers a real number rather than a zero this
// test invented — every arithmetic assertion below is in units of this
// cadence, so a kind picked here must keep one for as long as this file does.
const aScheduledKind = "capture_classify"

func TestAScheduledRunIsWhenThePassRuns(t *testing.T) {
	_, pool := migratedAppPool(t)
	ctx := t.Context()
	next := time.Now().Add(20 * time.Minute).Truncate(time.Second)

	// A completed run as well, and EARLIER: the scheduled row must win, or the
	// answer would be a projection standing in front of the real thing.
	seedJob(ctx, t, pool, seed{Kind: aScheduledKind, State: "completed"})
	seedJob(ctx, t, pool, seed{Kind: aScheduledKind, State: "scheduled", Scheduled: next})

	pass, err := jobs.PassFor(ctx, pool, aScheduledKind)
	if err != nil {
		t.Fatalf("reading the pass: %v", err)
	}
	if pass.NextAt == nil || !pass.NextAt.Equal(next) {
		t.Errorf("next = %v, want the scheduled row's own time %v", pass.NextAt, next)
	}
	if pass.Every != time.Hour {
		t.Errorf("cadence = %v, want the hour api/jobs.yaml declares", pass.Every)
	}
	if pass.Running {
		t.Error("no row is running and the read says one is")
	}
}

func TestWithNothingScheduledThePassIsTheLastRunPlusItsCadence(t *testing.T) {
	_, pool := migratedAppPool(t)
	ctx := t.Context()
	ran := time.Now().Add(-12 * time.Minute).Truncate(time.Second)

	// Two completed runs: the LATEST is the one the next pass follows, and a
	// read taking the earliest would name a time already past.
	//
	// Each is seeded with its scheduled_at APART from when it finished — the
	// helper's CreatedAt sets finalized_at and Scheduled sets the tick — because
	// the tick is what the interval measures from. A read that projected off the
	// finish would run late by however long the pass took, which for these two
	// kinds is up to the twenty minutes their timeout allows.
	seedJob(ctx, t, pool, seed{
		Kind: aScheduledKind, State: "completed",
		Scheduled: ran.Add(-time.Hour), CreatedAt: ran.Add(-time.Hour).Add(9 * time.Minute),
	})
	seedJob(ctx, t, pool, seed{
		Kind: aScheduledKind, State: "completed",
		Scheduled: ran, CreatedAt: ran.Add(9 * time.Minute),
	})

	pass, err := jobs.PassFor(ctx, pool, aScheduledKind)
	if err != nil {
		t.Fatalf("reading the pass: %v", err)
	}
	want := ran.Add(time.Hour)
	if pass.NextAt == nil || !pass.NextAt.Equal(want) {
		t.Errorf("next = %v, want the last TICK plus the cadence %v — a projection off the "+
			"finish runs late by however long the pass took", pass.NextAt, want)
	}
}

// A DUE RUN IS NOT A FUTURE ONE.
//
// River makes a row `available` when its moment arrives, and that row's
// scheduled_at is then in the PAST. Read as a next-run time it names a moment
// that has been and gone; read as nothing it falls through to a projection off
// the last completed run, which is a different lie. It is its own answer.
func TestADueRunIsReportedAsQueuedRatherThanAsATime(t *testing.T) {
	_, pool := migratedAppPool(t)
	ctx := t.Context()
	due := time.Now().Add(-3 * time.Minute)

	// A completed run as well, so a fall-through would have something to
	// project from — and would then print a time nobody is waiting for.
	seedJob(ctx, t, pool, seed{Kind: aScheduledKind, State: "completed", CreatedAt: due.Add(-time.Hour)})
	seedJob(ctx, t, pool, seed{Kind: aScheduledKind, State: "available", Scheduled: due})

	pass, err := jobs.PassFor(ctx, pool, aScheduledKind)
	if err != nil {
		t.Fatalf("reading the pass: %v", err)
	}
	if !pass.Queued {
		t.Error("a runnable row is reported as not queued, so the screen falls back to a " +
			"projection and names a time when what is really happening is a worker not picking work up")
	}
	if pass.Running {
		t.Error("a queued run is reported as running, which claims a worker has it")
	}
}

// A retry is a pass that is still coming, and reporting the cadence past it
// would promise a run at a time when what is pending is an attempt.
func TestARetryableRunCountsAsQueued(t *testing.T) {
	_, pool := migratedAppPool(t)
	ctx := t.Context()
	seedJob(ctx, t, pool, seed{
		Kind: aScheduledKind, State: "retryable", Scheduled: time.Now().Add(-time.Minute),
	})

	pass, err := jobs.PassFor(ctx, pool, aScheduledKind)
	if err != nil {
		t.Fatalf("reading the pass: %v", err)
	}
	if !pass.Queued {
		t.Error("a row waiting to be retried is reported as not queued")
	}
}

func TestARunningPassSaysSoRatherThanNamingATime(t *testing.T) {
	_, pool := migratedAppPool(t)
	ctx := t.Context()
	seedJob(ctx, t, pool, seed{Kind: aScheduledKind, State: "running"})

	pass, err := jobs.PassFor(ctx, pool, aScheduledKind)
	if err != nil {
		t.Fatalf("reading the pass: %v", err)
	}
	if !pass.Running {
		t.Error("a running row is reported as not running — queued and running are different " +
			"states to somebody watching a counter that has not moved")
	}
}

// A fleet whose last pass has aged out of River's retention, or one that has
// never run at all, answers NOTHING. Nil is not "soon": a caller owed a time
// this deployment cannot compute must say the cadence instead of inventing one.
func TestAKindWithNoHistoryNamesNoTime(t *testing.T) {
	_, pool := migratedAppPool(t)
	ctx := t.Context()

	pass, err := jobs.PassFor(ctx, pool, aScheduledKind)
	if err != nil {
		t.Fatalf("reading the pass: %v", err)
	}
	if pass.NextAt != nil {
		t.Errorf("next = %v for a kind with no scheduled and no completed run, want no answer",
			pass.NextAt)
	}
	if pass.Every != time.Hour {
		t.Errorf("cadence = %v — the declaration is knowable even where the next run is not", pass.Every)
	}
}

// A kind no clock runs answers a zero cadence rather than a made-up one, which
// is a different sentence from "the next run is unknown".
func TestAnUndeclaredKindHasNoCadence(t *testing.T) {
	_, pool := migratedAppPool(t)
	ctx := t.Context()

	pass, err := jobs.PassFor(ctx, pool, "a_kind_this_build_never_declared")
	if err != nil {
		t.Fatalf("reading the pass: %v", err)
	}
	if pass.Every != 0 {
		t.Errorf("cadence = %v for a kind api/jobs.yaml never declared", pass.Every)
	}
}

// A READ THAT FAILED IS AN ERROR, not an answer.
//
// Both callers render what this returns, and a zero Pass renders as "no clock
// runs this pass" — a sentence about the deployment, from a read that never
// reached the table. The kind is in the message because two surfaces ask about
// different ones and a log line that names neither cannot say which stopped.
func TestAFailedReadIsAnErrorRatherThanASilentAbsenceOfAClock(t *testing.T) {
	_, pool := migratedAppPool(t)
	stopped, cancel := context.WithCancel(t.Context())
	cancel()

	pass, err := jobs.PassFor(stopped, pool, aScheduledKind)
	if err == nil {
		t.Fatalf("a read on a cancelled context answered %+v and no error", pass)
	}
	if !strings.Contains(err.Error(), aScheduledKind) {
		t.Errorf("error = %q, want the kind named so a log line says which pass could not be read", err)
	}
	if pass != (jobs.Pass{}) {
		t.Errorf("pass = %+v beside an error, want the zero value — a caller that renders half a "+
			"failed read prints the cadence of a pass nobody asked the queue about", pass)
	}
}
