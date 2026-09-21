// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

// How long a connection has been failing, as opposed to what is wrong with it.
//
// A sync that postpones itself leaves its row scheduled in the future, so it is
// never late by any age reading — correctly, since nothing scheduled ahead is
// late — and River records no attempt error for a postponement. The class says
// WHAT, consecutive_failures counts ticks whose length is the backoff ladder's
// business, and last_synced_at moves on every one of them. Nothing said for how
// long, so a provider unreachable since breakfast and a mailbox idling between
// ticks read identically on a fleet screen.
//
// failing_since is that fact, and its whole content is that it does NOT move.
// These cases are the two halves of that: it survives the ticks after it, and a
// success ends the streak so the next failure starts a new one.
//
// The clock is injected because the property is only visible across instants
// far apart. Real time would need a sleep to show an hour, and a verdict that
// depends on elapsed wall time is the flakiness the suite refuses.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// haltingConnector fails until told otherwise, so one connection can run a
// streak and then recover on the same registry.
type haltingConnector struct {
	fixtureConnector
	recovered *bool
}

func (haltingConnector) Descriptor() connector.Descriptor {
	return connector.Descriptor{Name: haltingProvider, Version: "fixture"}
}

func (c haltingConnector) Sync(context.Context, connector.Auth, connector.Cursor, connector.Sink) (connector.Cursor, error) {
	if *c.recovered {
		return nil, nil
	}
	return nil, connector.ErrUnreachable
}

// haltingProvider is the connection every case here drives. The connector
// registers under it, and both readers below are keyed on it.
const haltingProvider = "test_mailbox"

// connectionOf reads back the id Connect wrote, so the ticks after the first
// drive the same connection rather than a second one.
func connectionOf(ctx context.Context, t *testing.T) ids.UUID {
	t.Helper()
	owner, _ := setupCaptureDB(t)
	var id ids.UUID
	if err := owner.QueryRow(ctx,
		`SELECT id FROM capture_connection WHERE provider = $1`, haltingProvider).Scan(&id); err != nil {
		t.Fatalf("reading back the %s connection: %v", haltingProvider, err)
	}
	return id
}

// streakState reads the two stamps that separate the streak from the tick.
func streakState(ctx context.Context, t *testing.T) (failingSince, lastSynced *time.Time) {
	t.Helper()
	owner, _ := setupCaptureDB(t)
	if err := owner.QueryRow(ctx, `
		SELECT s.failing_since, s.last_synced_at
		  FROM capture_sync_state s
		  JOIN capture_connection c ON c.id = s.connection_id
		 WHERE c.provider = $1`, haltingProvider).Scan(&failingSince, &lastSynced); err != nil {
		t.Fatalf("reading the streak state: %v", err)
	}
	return failingSince, lastSynced
}

func TestAPostponingConnectionReportsAGrowingFailureDuration(t *testing.T) {
	ctx, reg, _, _ := newCaptureRegistryFixture(t)
	recovered := false
	reg.Register(haltingConnector{recovered: &recovered})

	start := time.Date(2026, 5, 4, 8, 0, 0, 0, time.UTC)
	at := start
	reg.WithClock(func() time.Time { return at })

	if err := syncOnceOf(ctx, t, reg, haltingProvider); !errors.Is(err, connector.ErrUnreachable) {
		t.Fatalf("SyncOnce = %v, want the connector's own failure", err)
	}
	failingSince, _ := streakState(ctx, t)
	if failingSince == nil {
		t.Fatal("the first failure recorded no start — a streak nothing dates is one nothing can time")
	}
	if !failingSince.Equal(start) {
		t.Fatalf("the streak started at %s, want the instant of the first failure (%s)", failingSince, start)
	}

	// Two more ticks, an hour apart. This is the case that used to report
	// nothing: each one is a fresh attempt and the row was already failing.
	for _, tick := range []time.Duration{30 * time.Minute, time.Hour} {
		at = start.Add(tick)
		if err := reg.SyncOnce(ctx, connectionOf(ctx, t)); !errors.Is(err, connector.ErrUnreachable) {
			t.Fatalf("SyncOnce at +%v = %v, want the connector's own failure", tick, err)
		}
	}

	failingSince, lastSynced := streakState(ctx, t)
	if failingSince == nil || !failingSince.Equal(start) {
		t.Fatalf("after two more failures the streak starts at %v, want the first failure's instant (%s) — "+
			"a stamp that moves with the ticks reports the newest attempt and never an outage", failingSince, start)
	}
	// The tick DID move, which is what makes the assertion above mean
	// something: both stamps standing still would prove only that nothing wrote.
	if lastSynced == nil || !lastSynced.After(start) {
		t.Fatalf("the last-attempt stamp is %v, want it moved past the first failure — the writes under test did not happen", lastSynced)
	}
	// The reading the surfaces take: an hour, not the two minutes since the
	// last attempt.
	if got := at.Sub(*failingSince); got != time.Hour {
		t.Errorf("the duration a reader takes off the streak is %v, want 1h", got)
	}
}

func TestASuccessfulSyncEndsTheFailureStreak(t *testing.T) {
	ctx, reg, _, _ := newCaptureRegistryFixture(t)
	recovered := false
	reg.Register(haltingConnector{recovered: &recovered})

	start := time.Date(2026, 5, 4, 8, 0, 0, 0, time.UTC)
	at := start
	reg.WithClock(func() time.Time { return at })

	if err := syncOnceOf(ctx, t, reg, haltingProvider); !errors.Is(err, connector.ErrUnreachable) {
		t.Fatalf("SyncOnce = %v, want the connector's own failure", err)
	}
	connection := connectionOf(ctx, t)

	recovered = true
	at = start.Add(time.Hour)
	if err := reg.SyncOnce(ctx, connection); err != nil {
		t.Fatalf("SyncOnce after recovery: %v", err)
	}
	if failingSince, _ := streakState(ctx, t); failingSince != nil {
		t.Fatalf("a recovered connection still dates a streak at %s — one success ends it, or the next outage "+
			"would be reported as having run since this one", failingSince)
	}

	// And the NEXT failure starts a new streak from its own instant rather than
	// resuming the one that ended. A clear that only blanked the surface would
	// pass the assertion above and fail this one.
	recovered = false
	at = start.Add(3 * time.Hour)
	if err := reg.SyncOnce(ctx, connection); !errors.Is(err, connector.ErrUnreachable) {
		t.Fatalf("SyncOnce after relapse = %v, want the connector's own failure", err)
	}
	failingSince, _ := streakState(ctx, t)
	if failingSince == nil || !failingSince.Equal(at) {
		t.Fatalf("the new streak starts at %v, want the relapse's own instant (%s)", failingSince, at)
	}
}
