// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migration

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// runRowWrite matches a statement that updates import_run itself, not a
// column of another table named for it.
var runRowWrite = regexp.MustCompile(`(?i)\bUPDATE\s+import_run\b`)

// runWriteCounter counts the import_run updates one pool sends.
type runWriteCounter struct {
	mu    sync.Mutex
	count int
}

func (c *runWriteCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if runRowWrite.MatchString(data.SQL) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.count++
	}
	return ctx
}

func (*runWriteCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (c *runWriteCounter) total() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
}

// countedDB is the workspace ctx binds, over an app-role pool that counts
// its import_run updates.
func countedDB(ctx context.Context, t *testing.T, writes *runWriteCounter) *database.DB {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(os.Getenv("MARGINCE_TEST_APP_DSN"))
	if err != nil {
		t.Fatalf("parsing the app DSN: %v", err)
	}
	cfg.ConnConfig.Tracer = writes
	cfg.AfterConnect = func(_ context.Context, conn *pgx.Conn) error {
		database.RegisterIDTypes(conn)
		return nil
	}
	pool, err := testdb.OwnPoolFromConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("opening the counted pool: %v", err)
	}
	t.Cleanup(pool.Close)
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		t.Fatal("the test context binds no workspace")
	}
	return database.BindTo(pool, ids.From[ids.WorkspaceKind](ws))
}

func contactSource(n int) *fakeSource {
	rows := make([]Row, n)
	for i := range rows {
		rows[i] = Row{ExternalID: fmt.Sprintf("p-%d", i), Fields: map[string]any{"full_name": fmt.Sprintf("Contact %d", i)}}
	}
	return &fakeSource{order: []string{"contact"}, objects: map[string][]Row{"contact": rows}}
}

func TestASliceWritesTheRunRowOncePerFlushNotPerRow(t *testing.T) {
	ctx, db := testWorkspaceCtx(t, adminImportRunGrant())
	writes := &runWriteCounter{}
	counted := NewRunStore(countedDB(ctx, t, writes))
	const rows, flushRows = 500, 100
	// The completion is one transaction: its last flush, the status and the
	// release of the source file.
	const completion = 3

	leased := approvedRun(ctx, t, NewRunStore(db))
	if _, err := counted.Claim(ctx, leased); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	e := NewLeasedEngine(counted, leased, newMapWriters(counted, leased.Run))
	// A clock that never moves, so only rows decide when the slice flushes.
	stopped := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	e.now = func() time.Time { return stopped }
	if done, err := e.RunSlice(ctx, leased.Run, contactSource(rows), Budget{FlushRows: flushRows}); err != nil || !done {
		t.Fatalf("RunSlice = done %v, %v; want the run finished", done, err)
	}
	flushes := rows / flushRows
	if got, most := writes.total(), 1+flushes+completion; got > most || got < flushes {
		t.Errorf("a %d-row slice updated import_run %d times, want between its %d flushes and %d: "+
			"the claim, one per flush and the completion's", rows, got, flushes, most)
	}

	// The flip keeps its per-row checkpoint, which is what the counter must see.
	flipped := approvedRun(ctx, t, NewRunStore(db))
	before := writes.total()
	if _, err := NewEngine(counted, newMapWriters(counted, flipped.Run)).Run(ctx, flipped.Run, contactSource(rows)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := writes.total() - before; got < rows {
		t.Errorf("the flip updated import_run %d times for %d rows; the counter misses its per-row checkpoint", got, rows)
	}
}
