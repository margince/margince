// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The drift sweep has no timeout, so River's rescuer never reaches a row a
// killed worker left running. The session lock is what keeps two sweeps apart,
// and what lets the next sweep close the dead row.

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func seedSweepRow(ctx context.Context, t *testing.T, pool *pgxpool.Pool, state string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO river_job (state, kind, queue, priority, args, tags, errors, max_attempts, attempt, created_at, scheduled_at)
		VALUES ($1::river_job_state, $2, 'default', 1, '{}'::jsonb, '{}'::varchar(255)[], '{}'::jsonb[], 3, 1, now(), now())
		RETURNING id`, state, EmbedDriftSweepArgs{}.Kind()).Scan(&id); err != nil {
		t.Fatalf("seeding a %s sweep row: %v", state, err)
	}
	return id
}

func jobRowState(ctx context.Context, t *testing.T, pool *pgxpool.Pool, id int64) string {
	t.Helper()
	var state string
	if err := pool.QueryRow(ctx, `SELECT state::text FROM river_job WHERE id = $1`, id).Scan(&state); err != nil {
		t.Fatalf("reading job row %d: %v", id, err)
	}
	return state
}

func TestADriftSweepWhoseWorkerDiedDoesNotBlockTheNextOrLeaveItsRowRunning(t *testing.T) {
	e := integration.Setup(t)
	ctx := context.Background()
	w := &embedDriftSweepWorker{
		pool:     e.Pool,
		store:    search.NewStore(InstallationDB(e.Pool)),
		corpora:  knowledge.NewStore(InstallationDB(e.Pool)).WithBlobstore(blobstore.NewMemory()),
		embedder: &gatedEmbedder{},
		log:      slog.Default(),
	}
	identity, _ := w.embedder.EmbedIdentity()
	if err := w.store.SeedBinding(ctx, identity); err != nil {
		t.Fatalf("seeding the binding marker: %v", err)
	}
	dead := seedSweepRow(ctx, t, e.Pool, "running")
	own := seedSweepRow(ctx, t, e.Pool, "running")
	run := func() error {
		return w.Work(ctx, &river.Job[EmbedDriftSweepArgs]{JobRow: &rivertype.JobRow{ID: own, Kind: EmbedDriftSweepArgs{}.Kind()}})
	}

	release, held, err := holdSessionLock(ctx, e.Pool, embedDriftSweepLock, slog.Default())
	if err != nil || !held {
		t.Fatalf("taking the lock: held=%v err=%v", held, err)
	}
	t.Cleanup(release)
	if err := run(); err != nil {
		t.Fatalf("a sweep against a live holder: %v", err)
	}
	if got := jobRowState(ctx, t, e.Pool, dead); got != "running" {
		t.Fatalf("a sweep against a live holder closed the other row, now %s; the holder may be alive", got)
	}

	var killed int
	if err := e.Pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE pg_terminate_backend(pid, 5000))
		FROM pg_locks
		WHERE locktype = 'advisory' AND granted AND objsubid = 1
		  AND ((classid::bigint << 32) | (objid::bigint & 4294967295)) = hashtextextended($1, 0)`, embedDriftSweepLock).Scan(&killed); err != nil || killed != 1 {
		t.Fatalf("ending the holder's connection: ended=%d err=%v", killed, err)
	}

	if err := run(); err != nil {
		t.Fatalf("a sweep after the holder died: %v", err)
	}
	if got := jobRowState(ctx, t, e.Pool, dead); got != "discarded" {
		t.Errorf("the dead worker's row is %s, want discarded", got)
	}
	if got := jobRowState(ctx, t, e.Pool, own); got != "running" {
		t.Errorf("the sweep's own row is %s, want running until River finishes it", got)
	}
}

func TestADriftSweepTickNamesNoUniquenessSoADeadRowCannotAbsorbIt(t *testing.T) {
	opts := periodicInsertOpts(EmbedDriftSweepArgs{})
	if opts.UniqueOpts.ByArgs || len(opts.UniqueOpts.ByState) != 0 {
		t.Errorf("the drift sweep tick dedupes over %v, so a dead running row would absorb every later tick", opts.UniqueOpts.ByState)
	}
}

// poisonedPool answers a pool whose only idle connection the server has
// already ended. The client learns that on first use, as it would after a
// failover, so the statement fails after Acquire has succeeded.
func poisonedPool(t *testing.T, e *integration.Env) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	cfg, err := database.PoolConfig(os.Getenv("MARGINCE_TEST_APP_DSN"))
	if err != nil {
		t.Fatalf("parsing the pool config: %v", err)
	}
	// A pinging pool would notice the dead connection and replace it.
	cfg.ShouldPing = func(context.Context, pgxpool.ShouldPingParams) bool { return false }
	pool, err := testdb.OwnPoolFromConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("opening a pool of its own: %v", err)
	}
	t.Cleanup(pool.Close)
	var pid int
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquiring the connection to end: %v", err)
	}
	if err := conn.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatalf("reading its backend pid: %v", err)
	}
	conn.Release()
	if _, err := e.Pool.Exec(ctx, `SELECT pg_terminate_backend($1, 5000)`, pid); err != nil {
		t.Fatalf("ending the idle connection: %v", err)
	}
	return pool
}

func TestAHoldOnADeadConnectionFailsInsteadOfReadingAsAHeldLock(t *testing.T) {
	e := integration.Setup(t)
	ctx := context.Background()

	release, held, err := holdSessionLock(ctx, poisonedPool(t, e), embedDriftSweepLock, slog.Default())
	if err == nil || held || release != nil {
		t.Fatalf("hold = held %v, err %v; want a refusal, because a failed lock query is not a held lock", held, err)
	}
	if _, err := takeResetLock(ctx, poisonedPool(t, e), slog.Default()); err == nil || errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("a reset lock on a dead connection answered %v, want a failure that is not a conflict", err)
	}
	w := &embedDriftSweepWorker{pool: poisonedPool(t, e), log: slog.Default()}
	if err := w.Work(ctx, &river.Job[EmbedDriftSweepArgs]{JobRow: &rivertype.JobRow{ID: 1, Kind: EmbedDriftSweepArgs{}.Kind()}}); err == nil {
		t.Fatal("a sweep that could not ask for its lock answered nil, so River would record it as done")
	}
}
