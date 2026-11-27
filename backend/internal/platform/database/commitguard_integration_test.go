// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package database_test

import (
	"context"
	"errors"
	"math/rand/v2"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A COMMIT that has started finishes when its caller's context ends during it.
// Cut off, the row could land while the caller was told the save failed.
//
// A deferred trigger makes the COMMIT wait on an advisory lock this test holds.
// The test ends the context once Postgres shows the COMMIT waiting, then lets
// it go.
func TestACommitThatHasStartedFinishesWhenItsContextEnds(t *testing.T) {
	ctx, pool := laneEnv(t)
	wait, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	table, key := commitProbe(t, pool)
	holder, err := pool.Acquire(wait)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Release()
	if _, err := holder.Exec(wait, `SELECT pg_advisory_lock($1)`, key); err != nil {
		t.Fatal(err)
	}

	work, end := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- database.WithTx(work, pool, func(tx pgx.Tx) error {
			_, err := tx.Exec(work, `INSERT INTO `+table+` VALUES (1)`)
			return err
		})
	}()
	awaitCommitWaitingOnLock(wait, t, pool)
	end()
	if _, err := holder.Exec(wait, `SELECT pg_advisory_unlock($1)`, key); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("a COMMIT that had started answered %v, want nil", err)
		}
	case <-wait.Done():
		t.Fatal("the COMMIT never returned")
	}
	var rows int
	if err := pool.QueryRow(wait, `SELECT count(*) FROM `+table).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("rows = %d, want the committed 1", rows)
	}
}

// commitProbe creates a table whose deferred trigger takes an advisory lock at
// COMMIT, and answers the table and the lock key.
func commitProbe(t *testing.T, pool *pgxpool.Pool) (string, int64) {
	t.Helper()
	name := "commit_probe_" + ids.NewV7().String()[:8]
	table, fn := pgx.Identifier{name}.Sanitize(), pgx.Identifier{name + "_wait"}.Sanitize()
	key := rand.Int64()
	setup := []string{
		`CREATE TABLE ` + table + ` (k int)`,
		`CREATE FUNCTION ` + fn + `() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN PERFORM pg_advisory_xact_lock(TG_ARGV[0]::bigint); RETURN NULL; END $$`,
		`CREATE CONSTRAINT TRIGGER wait_at_commit AFTER INSERT ON ` + table +
			` DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION ` + fn +
			`('` + strconv.FormatInt(key, 10) + `')`,
	}
	t.Cleanup(func() {
		for _, drop := range []string{`DROP TABLE IF EXISTS ` + table, `DROP FUNCTION IF EXISTS ` + fn} {
			if _, err := pool.Exec(context.Background(), drop); err != nil {
				t.Errorf("%s: %v", drop, err)
			}
		}
	})
	for _, statement := range setup {
		if _, err := pool.Exec(context.Background(), statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	return table, key
}

// awaitCommitWaitingOnLock returns once a COMMIT on this database waits on an
// advisory lock.
func awaitCommitWaitingOnLock(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	for {
		var waiting bool
		err := pool.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock'
			  AND wait_event = 'advisory' AND query ILIKE 'commit%')`).Scan(&waiting)
		if err != nil {
			t.Fatalf("watching for the COMMIT to wait: %v", err)
		}
		if waiting {
			return
		}
	}
}

// A rollback after the caller's context ended still runs, so the pooled
// connection survives. Sent on the ended context it fails, and pgx closes the
// connection.
func TestARollbackOnAnEndedContextKeepsTheConnection(t *testing.T) {
	dsn := os.Getenv("MARGINCE_TEST_DSN")
	if dsn == "" {
		t.Fatal("MARGINCE_TEST_DSN is not set — run `make db-up` and try again")
	}
	pool, err := testdb.OwnPoolWith(context.Background(), dsn, func(cfg *pgxpool.Config) {
		cfg.MinConns, cfg.MaxConns = 0, 1
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	selectOne := func(tx pgx.Tx) error { _, err := tx.Exec(ctx, `SELECT 1`); return err }
	if err := database.WithTx(ctx, pool, selectOne); err != nil {
		t.Fatal(err)
	}
	dialled := pool.Stat().NewConnsCount()

	work, end := context.WithCancel(ctx)
	refused := errors.New("refused")
	if err := database.WithTx(work, pool, func(pgx.Tx) error { end(); return refused }); !errors.Is(err, refused) {
		t.Fatalf("the transaction answered %v, want its own refusal", err)
	}
	if err := database.WithTx(ctx, pool, selectOne); err != nil {
		t.Fatal(err)
	}

	if got := pool.Stat().NewConnsCount(); got != dialled {
		t.Errorf("the pool dialled %d new connections after the rollback, want 0", got-dialled)
	}
}

// A watched request learns that a write committed under it, and only a write:
// a refusal after a read changed nothing and may run again.
func TestAWatchedRequestLearnsOnlyOfCommittedWrites(t *testing.T) {
	ctx, pool := laneEnv(t)
	for name, statement := range map[string]string{
		"a read":  `SELECT 1`,
		"a write": `SELECT txid_current()`,
	} {
		t.Run(name, func(t *testing.T) {
			watched, committed := database.WatchCommits(ctx)
			if err := database.WithTx(watched, pool, func(tx pgx.Tx) error {
				_, err := tx.Exec(watched, statement)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if got, want := committed(), name == "a write"; got != want {
				t.Errorf("committed = %v, want %v", got, want)
			}
		})
	}
}
