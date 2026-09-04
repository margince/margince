// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package main

// What this proves, precisely: startEventLanes' FAILURE path hands back a value
// join() can use, join() returns, and it closes neither the bus nor the pool it
// was given. run() depends on all three — it defers lanes.join() before it checks
// the error, and the deferred closeBus and pool.Close run after that join.
//
// It needs a real bus and pool because with nil dependencies the lanes
// short-circuit and never start, so the failure path has nothing behind it.
//
// It does NOT prove that join() cancels and waits: whether a lane goroutine is
// still alive at any instant here is a scheduling race, and a subscriber that
// dies on its first bus call is indistinguishable from a healthy one at the
// moment it is launched. That property is proved deterministically, with real
// channels and no clock, by TestJoinCancelsTheLanesAndWaitsForThem.

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/overlaybudget/budgettest"
	"github.com/margince/margince/backend/internal/platform/testdb"
)

func TestABootFailureAfterALaneStartedStillJoinsIt(t *testing.T) {
	pool := workerTestPool(t)
	rdb := budgettest.Client(t)

	// announced takes only the phases' own Fprintln calls, which this goroutine
	// makes synchronously inside startEventLanes. The LANES get a discarding
	// logger: a subscriber that logs a bus hiccup would otherwise write this
	// buffer from its own goroutine while the assertion below reads it.
	var announced bytes.Buffer
	laneLog := slog.New(slog.NewTextHandler(io.Discard, nil))

	// The projection lanes start unconditionally; the webhook lane then refuses a
	// malformed signing key. So the failure lands AFTER lanes were launched, which
	// is the only interesting shape — a failure before any lane starts is
	// trivially safe to return from.
	// The lifetime is the CALLER's now, as it is in run(): created and its end
	// deferred before anything starts on it (#454). This test makes the same
	// two values run() makes, which is what keeps it a test of the boot shape
	// rather than of a shape only it uses.
	laneCtx, stopLanes := context.WithCancel(t.Context())
	var background sync.WaitGroup
	defer joinLanes(stopLanes, &background, laneLog)

	lanes, err := startEventLanes(laneCtx, &background, workerConfig{webhookKey: "not-a-valid-signing-key"},
		pool, rdb, nil, compose.ModelPath{}, laneLog, &announced)
	if err == nil {
		t.Fatal("startEventLanes accepted a malformed webhook signing key — this test needs it to fail AFTER a lane was launched")
	}
	// The regression this catches has MOVED rather than gone. It used to be a
	// `return workerLanes{}, err` making run()'s deferred join a nil
	// dereference; run() no longer joins through this value, so that shape can
	// no longer hurt it. What still matters is that the value carries the
	// caller's own group — a lane started here and counted somewhere else would
	// not be waited for by the caller's join.
	if lanes.background != &background {
		t.Fatal("a failing startEventLanes handed back a value carrying a different wait group than " +
			"the caller's; lanes counted there are lanes the caller's join never waits for")
	}
	// Evidence that a lane was launched before the failure, which is what makes
	// the failure path the interesting one. It claims only that: whether the
	// goroutine is still running is not knowable from here.
	if !strings.Contains(announced.String(), "interaction edges") {
		t.Errorf("the projection lane did not announce itself before the failure, so this run exercised a "+
			"failure with no lane behind it: %q", announced.String())
	}

	// Must RETURN. A join that waits on something nothing cancels hangs here
	// until the package timeout — no sleep and no clock of our own.
	joinLanes(stopLanes, &background, laneLog)

	// The join ends the lanes; it does not own the bus or the pool. run()'s deferred
	// closes do, and they run after it.
	if err := rdb.Ping(t.Context()).Err(); err != nil {
		t.Errorf("the lane join closed the bus client it was given: %v", err)
	}
	if err := pool.Ping(t.Context()); err != nil {
		t.Errorf("join() closed the pool it was given: %v", err)
	}
}

// workerTestPool opens the app-role pool the lanes consume through. It fails
// loudly rather than skipping — a boot-ordering gate that quietly does not run
// looks exactly like one that passed.
func workerTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("MARGINCE_TEST_APP_DSN")
	if dsn == "" {
		t.Fatal("MARGINCE_TEST_APP_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	pool, err := testdb.OwnPool(t.Context(), dsn)
	if err != nil {
		t.Fatalf("opening the app pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
