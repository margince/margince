// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package database_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/multitracer"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/testdb"
)

func holdThreadComponent(ctx context.Context, t *testing.T, pool *pgxpool.Pool, keys []string) func() {
	t.Helper()
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var workerErr error
	go func() {
		defer close(done)
		workerErr = database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
			if err := storekit.WaitThreadKeysTx(ctx, tx, keys); err != nil {
				return err
			}
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			close(release)
			<-done
			if workerErr != nil {
				t.Error(workerErr)
			}
		})
	}
	t.Cleanup(stop)
	select {
	case <-entered:
	case <-done:
		t.Fatalf("component owner: %v", workerErr)
	}
	return stop
}

func TestAComponentConflictAbortsAndRetriesTheWholeTransaction(t *testing.T) {
	ctx, pool := laneEnv(t)
	release := holdThreadComponent(ctx, t, pool, []string{"overlap"})
	attempts := 0
	err := storekit.RetryLockCycles(ctx, func() error {
		attempts++
		err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error { return storekit.LockThreadKeysTx(ctx, tx, "overlap") })
		if attempts == 1 {
			if !storekit.IsLockCycle(err) {
				t.Fatalf("contended component = %v, want native serialization failure", err)
			}
			release()
		}
		return err
	})
	if err != nil || attempts != 2 {
		t.Fatalf("retry: err=%v attempts=%d, want success after two", err, attempts)
	}
}

func TestASwallowedComponentConflictCannotCommit(t *testing.T) {
	ctx, pool := laneEnv(t)
	holdThreadComponent(ctx, t, pool, []string{"contended"})
	var conflict error
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		conflict = storekit.LockThreadKeysTx(ctx, tx, "contended")
		return nil
	})
	if !storekit.IsLockCycle(conflict) || err == nil {
		t.Fatalf("conflict=%v commit=%v, want aborted commit", conflict, err)
	}
}

func TestAWideComponentKeepsItsAdvisoryLocksBounded(t *testing.T) {
	ctx, pool := laneEnv(t)
	var keys []string
	for i := range 256 {
		keys = append(keys, fmt.Sprintf("wide-%d", i))
	}
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		if err := storekit.WaitThreadKeysTx(ctx, tx, keys); err != nil {
			return err
		}
		for _, key := range keys {
			if err := storekit.LockThreadKeysTx(ctx, tx, key); err != nil {
				return err
			}
		}
		var locks int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM pg_locks WHERE pid=pg_backend_pid() AND locktype='advisory'`).Scan(&locks); err != nil {
			return err
		}
		if locks != 1 {
			t.Errorf("wide component holds %d advisory locks, want one exclusive barrier", locks)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAnAmbientSnapshotLeavesComponentRetryToItsOwner(t *testing.T) {
	ctx, pool := laneEnv(t)
	attempts := 0
	err := database.WithWorkspaceSnapshot(ctx, pool, func(ctx context.Context) error {
		return storekit.RetryLockCycles(ctx, func() error {
			attempts++
			return database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error { return storekit.RetryThreadTransaction(ctx, tx) })
		})
	})
	if !storekit.IsLockCycle(err) || attempts != 1 {
		t.Fatalf("ambient retry: err=%v attempts=%d, want one aborted snapshot", err, attempts)
	}
}

func TestRepeatedThreadWritersShareOneTransactionLockBudget(t *testing.T) {
	ctx, pool := laneEnv(t)
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		for i := range 256 {
			if err := storekit.LockThreadKeysTx(ctx, tx, fmt.Sprintf("cumulative-%d", i)); err != nil {
				return err
			}
		}
		var locks int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM pg_locks WHERE pid=pg_backend_pid() AND locktype='advisory'`).Scan(&locks); err != nil {
			return err
		}
		if locks >= 256 {
			t.Errorf("cumulative writers retained %d locks instead of using the bounded barrier", locks)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAWideFrontierNeverQueuesAheadOfOrdinaryThreadWork(t *testing.T) {
	ctx, pool := laneEnv(t)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	release := holdThreadComponent(ctx, t, pool, []string{"active-component"})
	defer release()
	var keys []string
	for i := range 256 {
		keys = append(keys, fmt.Sprintf("wide-%d", i))
	}
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error { return storekit.WaitThreadKeysTx(ctx, tx, keys) })
	if !storekit.IsLockCycle(err) {
		t.Fatalf("busy wide frontier=%v, want transaction retry", err)
	}
	err = database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error { return storekit.LockThreadKeysTx(ctx, tx, "independent-component") })
	if err != nil {
		t.Fatalf("wide attempt blocked ordinary work: %v", err)
	}
}

func TestThreadComponentsReuseTheLegacyPrivacyMutex(t *testing.T) {
	ctx, pool := laneEnv(t)
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		if err := storekit.LockWriteIdentity(ctx, tx, "capture_thread_hold", "compatible-thread"); err != nil {
			return err
		}
		conflict := database.WithWorkspaceTx(ctx, pool, func(other pgx.Tx) error { return storekit.LockThreadKeysTx(ctx, other, "compatible-thread") })
		if !storekit.IsLockCycle(conflict) {
			t.Errorf("legacy privacy mutex did not fence component writer: %v", conflict)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestThreadLockOwnershipDoesNotSurviveSavepointRollbackOrCommit(t *testing.T) {
	ctx, pool := laneEnv(t)
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		savepoint, err := tx.Begin(ctx)
		if err != nil {
			return err
		}
		var keys []string
		for i := range 256 {
			keys = append(keys, fmt.Sprintf("savepoint-%d", i))
		}
		if err := storekit.WaitThreadKeysTx(ctx, savepoint, keys); err != nil {
			return err
		}
		if err := savepoint.Rollback(ctx); err != nil {
			return err
		}
		var locks int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM pg_locks WHERE pid=pg_backend_pid() AND locktype='advisory'`).Scan(&locks); err != nil {
			return err
		}
		if locks != 0 {
			t.Errorf("savepoint rollback retained %d locks", locks)
		}
		return storekit.LockThreadKeysTx(ctx, tx, "after-rollback")
	})
	if err != nil {
		t.Fatal(err)
	}
	err = database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		if err := storekit.LockThreadKeysTx(ctx, tx, "after-rollback"); err != nil {
			return err
		}
		var locks int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM pg_locks WHERE pid=pg_backend_pid() AND locktype='advisory'`).Scan(&locks); err != nil {
			return err
		}
		if locks != 2 {
			t.Errorf("new transaction acquired %d locks, want its own component and barrier", locks)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

type cancelComponentWait struct{ cancel context.CancelFunc }

func (c cancelComponentWait) TraceQueryStart(ctx context.Context, _ *pgx.Conn, q pgx.TraceQueryStartData) context.Context {
	if strings.Contains(q.SQL, "pg_advisory_xact_lock(key)") {
		c.cancel()
	}
	return ctx
}
func (cancelComponentWait) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestCancellationDuringComponentAcquisitionReleasesItsBarrier(t *testing.T) {
	ctx, pool := laneEnv(t)
	release := holdThreadComponent(ctx, t, pool, []string{"cancelled-component"})
	waiting, cancel := context.WithCancel(ctx)
	defer cancel()
	config := pool.Config()
	config.ConnConfig.Tracer = multitracer.New(config.ConnConfig.Tracer, cancelComponentWait{cancel: cancel})
	config.ConnConfig.DialFunc = lateClosingDial(t, config.ConnConfig.DialFunc)
	waiter, err := testdb.OwnPoolFromConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer waiter.Close()
	err = database.WithWorkspaceTx(waiting, waiter, func(tx pgx.Tx) error { return storekit.WaitThreadKeysTx(waiting, tx, []string{"cancelled-component"}) })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait returned %v", err)
	}
	// Before the holder lets go: a waiter whose transaction outlived the cancel
	// would still hold the barrier it took before the component.
	awaitBarrierHolders(ctx, t, pool, 1)
	release()
	var keys []string
	for i := range 256 {
		keys = append(keys, fmt.Sprintf("after-cancel-%d", i))
	}
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error { return storekit.WaitThreadKeysTx(ctx, tx, keys) }); err != nil {
		t.Fatalf("cancelled acquisition leaked its barrier: %v", err)
	}
}

// awaitBarrierHolders waits until want backends in this database hold the
// thread barrier. A cancelled waiter's backend lets go once it reads pgx's
// Terminate, which is prompt but not instant.
func awaitBarrierHolders(ctx context.Context, t *testing.T, pool *pgxpool.Pool, want int) {
	t.Helper()
	deadline, stop := context.WithTimeout(ctx, 10*time.Second)
	defer stop()
	var holders int
	for deadline.Err() == nil {
		if err := pool.QueryRow(deadline, `SELECT count(DISTINCT pid) FROM pg_locks
			WHERE locktype = 'advisory' AND objsubid = 1 AND granted
			  AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
			  AND ((classid::bigint << 32) | objid::bigint) = hashtextextended('thread_merge', 0)`).Scan(&holders); err != nil {
			if deadline.Err() != nil {
				break
			}
			t.Fatalf("reading who holds the thread barrier: %v", err)
		}
		if holders == want {
			return
		}
	}
	t.Fatalf("%d backends still hold the thread barrier, want %d: a cancelled wait kept it", holders, want)
}

// lateClosingDial sends each Terminate 300 ms late. The waiter's backend sits
// idle in its transaction and lets go only on that Terminate. So the window CI
// hit by chance is open on every run. A CancelRequest is forwarded at once;
// neither test proves pgx sent one.
func lateClosingDial(t *testing.T, dial pgconn.DialFunc) pgconn.DialFunc {
	t.Helper()
	pending := &sync.WaitGroup{}
	t.Cleanup(pending.Wait)
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := dial(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		return &lateClosingConn{Conn: conn, t: t, pending: pending}, nil
	}
}

// terminateMessage is the frontend Terminate: type X and a length of 4.
var terminateMessage = []byte{'X', 0, 0, 0, 4}

type lateClosingConn struct {
	net.Conn
	t          *testing.T
	pending    *sync.WaitGroup
	once       sync.Once
	terminated atomic.Bool
}

func (c *lateClosingConn) Write(p []byte) (int, error) {
	if bytes.Equal(p, terminateMessage) {
		c.terminated.Store(true)
		return len(p), nil
	}
	return c.Conn.Write(p)
}

// Close returns at once and says goodbye later. A failure there is logged: the
// server then learns of the closed connection the ordinary way.
func (c *lateClosingConn) Close() error {
	c.once.Do(func() {
		c.pending.Add(1)
		time.AfterFunc(300*time.Millisecond, func() {
			defer c.pending.Done()
			if c.terminated.Load() {
				if err := c.SetWriteDeadline(time.Time{}); err != nil {
					c.t.Logf("clearing the delayed socket's deadline: %v", err)
				} else if _, err := c.Conn.Write(terminateMessage); err != nil && !errors.Is(err, net.ErrClosed) {
					c.t.Logf("sending the delayed Terminate: %v", err)
				}
			}
			if err := c.Conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
				c.t.Logf("closing the delayed socket: %v", err)
			}
		})
	})
	return nil
}
