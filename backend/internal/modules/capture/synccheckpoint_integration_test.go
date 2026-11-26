// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// checkpointingConnector is a first sync that saves one checkpoint, lets the
// test move the connection on, then tries a second. What the row holds after
// the first save is read inside the round, before the sync returns.
type checkpointingConnector struct {
	anchorRecorder
	afterFirst  func()
	secondSaved error
}

func (c *checkpointingConnector) SyncFrom(
	ctx context.Context, _ connector.Auth, _ connector.Cursor, _ time.Time, _ connector.Sink,
) (connector.Cursor, error) {
	if err := connector.CheckpointSync(ctx, connector.Cursor(`{"round":"first"}`)); err != nil {
		return nil, err
	}
	c.afterFirst()
	c.secondSaved = connector.CheckpointSync(ctx, connector.Cursor(`{"round":"second"}`))
	if c.secondSaved != nil {
		return nil, c.secondSaved
	}
	return connector.Cursor(`{"round":"closed"}`), nil
}

// connectionRow is what a checkpoint may and may not change.
type connectionRow struct {
	cursor    string
	anchored  bool
	succeeded bool
}

func readConnectionRow(ctx context.Context, t *testing.T, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
},
) connectionRow {
	t.Helper()
	var row connectionRow
	if err := db.QueryRow(ctx, `
		SELECT coalesce(c.sync_cursor::text, ''), c.sync_anchor IS NOT NULL,
		       coalesce(s.last_success_at IS NOT NULL, false)
		  FROM capture_connection c LEFT JOIN capture_sync_state s ON s.connection_id = c.id
		 WHERE c.provider = $1`, anchorProvider).Scan(&row.cursor, &row.anchored, &row.succeeded); err != nil {
		t.Fatalf("reading the connection: %v", err)
	}
	return row
}

// A checkpoint stores the cursor and nothing else: the first sync's anchor
// stays held and no success is recorded until the round closes.
func TestACheckpointStoresTheCursorButNotASuccess(t *testing.T) {
	ctx, reg, _, _ := newCaptureRegistryFixture(t)
	owner, _ := setupCaptureDB(t)
	reg.SetClock(func() time.Time { return time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC) })
	var mid connectionRow
	rec := &checkpointingConnector{}
	rec.window = 7 * 24 * time.Hour
	rec.afterFirst = func() { mid = readConnectionRow(ctx, t, owner) }
	reg.Register(rec)

	if err := syncOnceOf(ctx, t, reg, anchorProvider); err != nil {
		t.Fatalf("the sync failed: %v", err)
	}
	if mid.cursor != `{"round": "first"}` || !mid.anchored || mid.succeeded {
		t.Errorf("mid-round the row held %+v, want the checkpoint, the anchor held and no success", mid)
	}
	if end := readConnectionRow(ctx, t, owner); end.cursor != `{"round": "closed"}` || end.anchored || !end.succeeded {
		t.Errorf("after the round the row held %+v, want the closed cursor, the anchor released and a success", end)
	}
}

// A sync whose connection moved on mid-round stops at its last checkpoint. It
// returns no error, overwrites nothing and records no failure.
func TestASupersededCheckpointStopsTheSyncQuietly(t *testing.T) {
	ctx, reg, _, _ := newCaptureRegistryFixture(t)
	owner, _ := setupCaptureDB(t)
	reg.SetClock(func() time.Time { return time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC) })
	rec := &checkpointingConnector{}
	rec.window = 7 * 24 * time.Hour
	rec.afterFirst = func() {
		if _, err := owner.Exec(ctx,
			`UPDATE capture_connection SET sync_revision = sync_revision + 1 WHERE provider = $1`, anchorProvider); err != nil {
			t.Fatalf("moving the connection on: %v", err)
		}
	}
	reg.Register(rec)

	if err := syncOnceOf(ctx, t, reg, anchorProvider); err != nil {
		t.Fatalf("a superseded sync answered %v, want it to stop quietly", err)
	}
	if rec.secondSaved == nil {
		t.Fatal("the checkpoint after the connection moved on was saved")
	}
	end := readConnectionRow(ctx, t, owner)
	if end.cursor != `{"round": "first"}` || !end.anchored || end.succeeded {
		t.Errorf("the row held %+v, want the last checkpoint, the anchor held and no success", end)
	}
	var failures int
	if err := owner.QueryRow(ctx, `
		SELECT coalesce(max(s.consecutive_failures), 0) FROM capture_sync_state s
		  JOIN capture_connection c ON c.id = s.connection_id WHERE c.provider = $1`, anchorProvider).Scan(&failures); err != nil {
		t.Fatalf("reading the failure count: %v", err)
	}
	if failures != 0 {
		t.Errorf("a superseded sync counted %d failures", failures)
	}
}

// interruptedFirstSync checkpoints its first attempt and then fails, as a
// first sync cut off mid-round does. Every attempt records what it was handed.
type interruptedFirstSync struct {
	anchorRecorder
	cursors []string
}

func (c *interruptedFirstSync) SyncFrom(
	ctx context.Context, _ connector.Auth, cursor connector.Cursor, from time.Time, _ connector.Sink,
) (connector.Cursor, error) {
	c.told = append(c.told, from)
	c.cursors = append(c.cursors, string(cursor))
	if len(c.told) > 1 {
		return connector.Cursor(`{"round":"closed"}`), nil
	}
	if err := connector.CheckpointSync(ctx, connector.Cursor(`{"round":"first"}`)); err != nil {
		return nil, err
	}
	return nil, connector.ErrUnreachable
}

// A first sync resumed from its checkpoint reads from the anchor it held, for
// as long as that anchor binds.
func TestAResumedFirstSyncReadsFromItsHeldAnchor(t *testing.T) {
	ctx, reg, _, _ := newCaptureRegistryFixture(t)
	at := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	reg.SetClock(func() time.Time { return at })
	rec := &interruptedFirstSync{}
	rec.window = 7 * 24 * time.Hour
	reg.Register(rec)

	if err := syncOnceOf(ctx, t, reg, anchorProvider); err == nil {
		t.Fatal("the interrupted first attempt succeeded")
	}
	at = at.Add(48 * time.Hour)
	if err := syncOnceOfExisting(ctx, t, reg, anchorProvider); err != nil {
		t.Fatalf("the resumed sync failed: %v", err)
	}
	if len(rec.told) != 2 {
		t.Fatalf("the connector was told where to start %d times, want 2", len(rec.told))
	}
	if rec.cursors[1] != `{"round": "first"}` || !rec.told[1].Equal(rec.told[0]) {
		t.Errorf("the resume was handed %q from %s, want the checkpoint from the held %s", rec.cursors[1], rec.told[1], rec.told[0])
	}
}

// Past anchorBinds a checkpointed first sync goes the way of any lapsed
// anchor. Its round is dropped, the period nobody read is recorded, and the
// sync starts again from its own window.
func TestALapsedCheckpointedFirstSyncRecordsWhatNobodyRead(t *testing.T) {
	ctx, reg, _, _ := newCaptureRegistryFixture(t)
	owner, _ := setupCaptureDB(t)
	at := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	reg.SetClock(func() time.Time { return at })
	rec := &interruptedFirstSync{}
	rec.window = 7 * 24 * time.Hour
	reg.Register(rec)

	if err := syncOnceOf(ctx, t, reg, anchorProvider); err == nil {
		t.Fatal("the interrupted first attempt succeeded")
	}
	held := rec.told[0]
	at = at.Add(200 * 24 * time.Hour)
	if err := syncOnceOfExisting(ctx, t, reg, anchorProvider); err != nil {
		t.Fatalf("the resumed sync failed: %v", err)
	}

	window := at.Add(-rec.window)
	if len(rec.told) != 2 {
		t.Fatalf("the connector was told where to start %d times, want 2", len(rec.told))
	}
	if rec.cursors[1] != "" || !rec.told[1].Equal(window) {
		t.Errorf("the resume was handed %q from %s, want no cursor from its own window %s", rec.cursors[1], rec.told[1], window)
	}
	var from, to time.Time
	if err := owner.QueryRow(ctx,
		`SELECT unread_from, unread_to FROM capture_connection WHERE provider = $1`, anchorProvider).
		Scan(&from, &to); err != nil {
		t.Fatalf("reading the recorded period: %v", err)
	}
	if !from.Equal(held) || !to.Equal(window) {
		t.Errorf("the unread period is %s to %s, want %s to %s", from, to, held, window)
	}
	if end := readConnectionRow(ctx, t, owner); end.cursor != `{"round": "closed"}` || end.anchored || !end.succeeded {
		t.Errorf("after the fresh round the row held %+v, want its cursor stored, the anchor released and a success", end)
	}
}
