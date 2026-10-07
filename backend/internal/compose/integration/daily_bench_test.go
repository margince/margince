// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// TestDailyUseBudgets times the screens a rep and a manager open every day,
// over HTTP, as those seats. Its subtests share state and run in order:
// seats, then seed, cold, latency and load as the suite grows; run it whole.
func TestDailyUseBudgets(t *testing.T) {
	scale, err := dailyScale()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("daily bench at scale %g", scale)
	e := apptest.SetupApp(t)
	// Without a statement cache nothing is prepared, so the generic-plan
	// regressions this bench exists to catch could never appear in a sample.
	if mode := e.Pool.Config().ConnConfig.DefaultQueryExecMode; mode != pgx.QueryExecModeCacheStatement {
		t.Fatalf("the app pool prepares statements in mode %q, want cache_statement: run through make bench-daily, which sets default_query_exec_mode=cache_statement", mode)
	}
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)

	var seats Seats
	var corpus DailyCorpus
	t.Run("seats", func(t *testing.T) {
		seats = createDailySeats(t, e)
		if len(seats.Managers) != 2 || len(seats.Reps) != 2*dailyRepsPerTeam {
			t.Fatalf("signed in %d managers and %d reps, want 2 and %d", len(seats.Managers), len(seats.Reps), 2*dailyRepsPerTeam)
		}
		t.Logf("seats: %d signed in (%d managers, %d reps)", len(seats.Managers)+len(seats.Reps), len(seats.Managers), len(seats.Reps))
	})

	t.Run("seed", func(t *testing.T) {
		start := time.Now()
		corpus = seedDailyCorpus(t, e, seats, scale)
		t.Logf("seed: corpus at scale %g in %s; flows open deals %q and companies %q",
			scale, time.Since(start).Round(time.Second), corpus.DealNames, corpus.CompanyNames)
		assertDailyContactScope(t, e, seats, corpus)
	})
}
