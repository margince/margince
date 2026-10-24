// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// A first sync that fails keeps the point it was going to read from.
//
// Derived fresh on each attempt, the retry anchors later than the one before
// it. The mail between the two is read by neither: too late for the second,
// never reached by the first.

// anchorRecorder is a connector whose first pull is time-bounded and which
// records where it was told to start. It fails until `failUntil` attempts have
// been made, which is how an outage of a chosen length is written without a
// clock that waits.
type anchorRecorder struct {
	fixtureConnector
	window    time.Duration
	failUntil int
	attempts  int
	told      []time.Time
}

// anchorProvider is in the connection table's provider CHECK and is taken by
// no other suite here. Register panics on a duplicate, and the fixture this
// embeds answers "gmail", which another suite already holds.
const anchorProvider = "test_mailbox"

func (c *anchorRecorder) Descriptor() connector.Descriptor {
	d := c.fixtureConnector.Descriptor()
	d.Name = anchorProvider
	d.FirstSyncWindow = c.window
	return d
}

func (c *anchorRecorder) SyncFrom(
	_ context.Context, _ connector.Auth, cursor connector.Cursor,
	from time.Time, _ connector.Sink,
) (connector.Cursor, error) {
	c.attempts++
	c.told = append(c.told, from)
	if c.attempts <= c.failUntil {
		return nil, connector.ErrUnreachable
	}
	return connector.Cursor(`{"watermark":"after-the-outage"}`), nil
}

// A short outage loses no mail: the retry reads from where the first attempt
// was going to, not from a later point.
func TestARetryOfAFirstSyncReadsWhereTheFirstAttemptDid(t *testing.T) {
	ctx, reg, _, _ := newCaptureRegistryFixture(t)
	at := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	reg.WithClock(func() time.Time { return at })
	rec := &anchorRecorder{window: 7 * 24 * time.Hour, failUntil: 1}
	reg.Register(rec)

	if err := syncOnceOf(ctx, t, reg, anchorProvider); !errors.Is(err, connector.ErrUnreachable) {
		t.Fatalf("the first attempt answered %v, want it to fail so a retry follows", err)
	}
	// Two days later, which is inside the window and inside the bound.
	at = at.Add(48 * time.Hour)
	if err := syncOnceOfExisting(ctx, t, reg, anchorProvider); err != nil {
		t.Fatalf("the retry failed: %v", err)
	}

	if len(rec.told) != 2 {
		t.Fatalf("the connector was told %d times, want 2", len(rec.told))
	}
	if !rec.told[1].Equal(rec.told[0]) {
		t.Errorf("the retry read from %s and the first attempt from %s: the mail between is read by neither",
			rec.told[1], rec.told[0])
	}
}

// An outage past the bound records the skipped window, and reads from its own
// window again rather than streaming everything since the held point.
func TestAnOutagePastTheBoundRecordsWhatNobodyRead(t *testing.T) {
	ctx, reg, _, _ := newCaptureRegistryFixture(t)
	owner, _ := setupCaptureDB(t)
	at := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	reg.WithClock(func() time.Time { return at })
	const window = 7 * 24 * time.Hour
	rec := &anchorRecorder{window: window, failUntil: 1}
	reg.Register(rec)

	if err := syncOnceOf(ctx, t, reg, anchorProvider); !errors.Is(err, connector.ErrUnreachable) {
		t.Fatalf("the first attempt answered %v, want it to fail", err)
	}
	held := rec.told[0]

	// Past the bound: the held point no longer binds.
	at = at.Add(200 * 24 * time.Hour)
	if err := syncOnceOfExisting(ctx, t, reg, anchorProvider); err != nil {
		t.Fatalf("the retry failed: %v", err)
	}

	wantFrom, wantTo := held, at.Add(-window)
	var from, to time.Time
	if err := owner.QueryRow(ctx,
		`SELECT unread_from, unread_to FROM capture_connection WHERE provider = $1`, anchorProvider).
		Scan(&from, &to); err != nil {
		t.Fatalf("reading the recorded period: %v", err)
	}
	if !from.Equal(wantFrom) {
		t.Errorf("the period starts %s, want the point the first attempt held, %s", from, wantFrom)
	}
	if !to.Equal(wantTo) {
		t.Errorf("the period ends %s, want where the retry had to start, %s", to, wantTo)
	}
	// And the retry read from its own window, not from the expired point: an
	// anchor held for 200 days would stream 200 days.
	if !rec.told[1].Equal(wantTo) {
		t.Errorf("the retry read from %s, want %s", rec.told[1], wantTo)
	}
}

// A sync that stored a watermark lets the anchor go. Held past that, a later
// re-anchor would read a stale lower bound as a point somebody chose.
func TestAStoredWatermarkReleasesTheAnchor(t *testing.T) {
	ctx, reg, _, _ := newCaptureRegistryFixture(t)
	owner, _ := setupCaptureDB(t)
	at := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	reg.WithClock(func() time.Time { return at })
	reg.Register(&anchorRecorder{window: 7 * 24 * time.Hour})

	if err := syncOnceOf(ctx, t, reg, anchorProvider); err != nil {
		t.Fatalf("the sync failed: %v", err)
	}

	var held *time.Time
	if err := owner.QueryRow(ctx,
		`SELECT sync_anchor FROM capture_connection WHERE provider = $1`, anchorProvider).Scan(&held); err != nil {
		t.Fatalf("reading the anchor back: %v", err)
	}
	if held != nil {
		t.Errorf("the anchor is still held at %s after a watermark was stored", *held)
	}
}

// syncOnceOfExisting drives another sync of a connection Connect already made,
// which is what a retry is. syncOnceOf connects first and would refuse a
// second time.
func syncOnceOfExisting(ctx context.Context, t *testing.T, reg interface {
	SyncOnce(context.Context, ids.UUID) error
}, provider string,
) error {
	t.Helper()
	owner, _ := setupCaptureDB(t)
	var id ids.UUID
	if err := owner.QueryRow(ctx,
		`SELECT id FROM capture_connection WHERE provider = $1`, provider).Scan(&id); err != nil {
		t.Fatalf("reading the connection back: %v", err)
	}
	return reg.SyncOnce(ctx, id)
}

// An anchor belongs to a first sync. A connection with a watermark already
// stored reads from its cursor instead. Holding one past that point costs two
// writes a sync and hands the bounded re-anchor a stale point. Worse, a long
// outage would tell the mailbox owner mail went unread that the cursor was
// about to bring in.
func TestAConnectionWithAStoredWatermarkIsToldNoAnchor(t *testing.T) {
	ctx, reg, _, _ := newCaptureRegistryFixture(t)
	owner, _ := setupCaptureDB(t)
	at := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	reg.WithClock(func() time.Time { return at })
	rec := &anchorRecorder{window: 7 * 24 * time.Hour}
	reg.Register(rec)

	// The first sync stores a watermark, which is what makes the second one
	// incremental.
	if err := syncOnceOf(ctx, t, reg, anchorProvider); err != nil {
		t.Fatalf("the first sync failed: %v", err)
	}
	reg.WithClock(func() time.Time { return at.Add(200 * 24 * time.Hour) })
	if err := syncOnceOfExisting(ctx, t, reg, anchorProvider); err != nil {
		t.Fatalf("the second sync failed: %v", err)
	}

	// One anchored read, for the first sync. The second took the plain Sync
	// path, which is where a cursor-bearing sync belongs.
	if len(rec.told) != 1 {
		t.Errorf("the connector was told an anchor %d times, want 1 (the first sync)", len(rec.told))
	}
	var held, from, to *time.Time
	if err := owner.QueryRow(ctx,
		`SELECT sync_anchor, unread_from, unread_to
		   FROM capture_connection WHERE provider = $1`, anchorProvider).Scan(&held, &from, &to); err != nil {
		t.Fatalf("reading the row back: %v", err)
	}
	if held != nil {
		t.Errorf("an incremental sync holds an anchor at %s", *held)
	}
	if from != nil || to != nil {
		t.Errorf("an incremental sync recorded an unread period of %v to %v", from, to)
	}
}
