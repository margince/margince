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
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/jobs"
)

// aScheduledKind is a kind api/jobs.yaml declares with a fixed cadence, so the
// spec lookup answers a real number rather than a zero this test invented.
const aScheduledKind = "capture_counterparty_verdict"

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
	seedJob(ctx, t, pool, seed{Kind: aScheduledKind, State: "completed", CreatedAt: ran.Add(-time.Hour)})
	seedJob(ctx, t, pool, seed{Kind: aScheduledKind, State: "completed", CreatedAt: ran})

	pass, err := jobs.PassFor(ctx, pool, aScheduledKind)
	if err != nil {
		t.Fatalf("reading the pass: %v", err)
	}
	want := ran.Add(time.Hour)
	if pass.NextAt == nil || !pass.NextAt.Equal(want) {
		t.Errorf("next = %v, want the last run plus the cadence %v", pass.NextAt, want)
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
