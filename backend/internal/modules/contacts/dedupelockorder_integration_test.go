// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The contact writers' lock order (lockorder.go), held between real writers. A
// real create is frozen with its address-base lock held. Two more real writers
// queue behind it in a chosen order, and the release must end with no deadlock.
// No sleep decides the order: each writer starts only once the one before it is
// seen waiting.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// heldCreate is a real create whose transaction stays open until released.
type heldCreate struct {
	pid     int
	release chan struct{}
	done    chan error
}

// holdCreate runs Store.CreateContactTx for one address and holds the
// transaction open, with every lane lock the create took, until release.
func (e *dedupeEnv) holdCreate(t *testing.T, name, email string) *heldCreate {
	t.Helper()
	ctx := e.as()
	h := &heldCreate{release: make(chan struct{}), done: make(chan error, 1)}
	ready := make(chan int, 1)
	go func() {
		h.done <- e.store.tx(ctx, func(tx pgx.Tx) error {
			var pid int
			if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				return err
			}
			if _, err := e.store.CreateContactTx(ctx, tx, CreateContactInput{
				FullName: name, Source: "manual",
				Emails: []ContactEmailInput{{Email: email, EmailType: "work", IsPrimary: true}},
			}, CustomColumns{}); err != nil {
				return err
			}
			ready <- pid
			<-h.release
			return nil
		})
	}()
	select {
	case h.pid = <-ready:
	case err := <-h.done:
		t.Fatalf("the held create failed: %v", err)
	}
	t.Cleanup(func() {
		select {
		case <-h.release:
		default:
			close(h.release)
		}
	})
	return h
}

// finish releases the held create and waits for its commit.
func (h *heldCreate) finish(t *testing.T) {
	t.Helper()
	close(h.release)
	if err := <-h.done; err != nil {
		t.Fatalf("the held create: %v", err)
	}
}

// waitForWaiters returns once n writers are waiting behind the held create, or
// fails when the budget runs out first.
func (e *dedupeEnv) waitForWaiters(t *testing.T, h *heldCreate, n int) {
	t.Helper()
	if finished, err := e.waitForWaitersOrDone(t, h, n, nil); finished {
		t.Fatalf("a writer finished while it was expected to wait: %v", err)
	}
}

// waitForWaitersOrDone is waitForWaiters for a writer that may finish without
// waiting at all: it reports the writer's error once it finishes first.
func (e *dedupeEnv) waitForWaitersOrDone(t *testing.T, h *heldCreate, n int, done <-chan error) (bool, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		waiting := countWaiters(ctx, t, e.store.db.Pool(), h.pid)
		if waiting >= n {
			return false, nil
		}
		select {
		case err := <-done:
			return true, err
		case <-ctx.Done():
			t.Fatalf("only %d of %d writers waited behind the held create", waiting, n)
		case <-tick.C:
		}
	}
}

// countWaiters counts the backends in this test database waiting on a lock,
// directly behind the held create or behind a writer queued after it. The
// connection is held only for the probe, so the writers keep the pool. The
// statistics snapshot is cleared on that same connection first.
func countWaiters(ctx context.Context, t *testing.T, pool *pgxpool.Pool, pid int) int {
	t.Helper()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquiring the probe connection: %v", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
		t.Fatalf("clearing the statistics snapshot: %v", err)
	}
	var waiting int
	if err := conn.QueryRow(ctx, `
		SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND cardinality(pg_blocking_pids(pid)) > 0
		   AND pid <> $1`, pid).Scan(&waiting); err != nil {
		t.Fatalf("probing for waiters: %v", err)
	}
	return waiting
}

// inBackground runs one writer and hands its error back.
func inBackground(write func() error) <-chan error {
	done := make(chan error, 1)
	go func() { done <- write() }()
	return done
}

// assertNoDeadlock fails when a writer was aborted to break a lock cycle.
func assertNoDeadlock(t *testing.T, who string, err error) {
	t.Helper()
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "40P01" {
		t.Fatalf("%s was aborted by a deadlock: the writers took their locks in opposite orders", who)
	}
}

// A create waits first, then an edit giving another contact the same address.
// The create commits, and the edit is refused the claimed address.
func TestAnEditAndACreateOfOneAddressNeverDeadlock(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	edited := ids.From[ids.ContactKind](e.plainContact(ctx, t, "Xavier Edit", "xavier@elsewhere.test"))
	held := e.holdCreate(t, "Probe Holder", "ab+zz@example.com")

	create := inBackground(func() error {
		_, err := e.store.CreateContact(e.as(), CreateContactInput{
			FullName: "Bea Create", Source: "manual",
			Emails: []ContactEmailInput{{Email: "ab+x@example.com", EmailType: "work", IsPrimary: true}},
		})
		return err
	})
	e.waitForWaiters(t, held, 1)
	edit := inBackground(func() error {
		_, err := e.store.UpdateContact(e.as(), edited, UpdateContactInput{
			Emails: []ContactEmailInput{{Email: "ab+x@example.com", EmailType: "work", IsPrimary: true}},
		})
		return err
	})
	e.waitForWaiters(t, held, 2)
	held.finish(t)

	createErr, editErr := <-create, <-edit
	assertNoDeadlock(t, "the create", createErr)
	assertNoDeadlock(t, "the edit", editErr)
	if createErr != nil {
		t.Fatalf("the create, first in line, failed: %v", createErr)
	}
	if editErr == nil {
		t.Fatal("the edit took an address the create had already claimed")
	}
}

// Capture lands mail on an exact incumbent while an edit gives that incumbent
// a second address under the same base. Both complete.
func TestCaptureOnAnIncumbentAndAnEditOfItNeverDeadlock(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	incumbent := ids.From[ids.ContactKind](e.plainContact(ctx, t, "Ab", "ab@example.com"))
	in := e.datedEnsureInput(ctx, t, "ab@example.com", "example.com", nowInTx(ctx, t, e))
	held := e.holdCreate(t, "Probe Holder", "ab+zz@example.com")

	capture := inBackground(func() error {
		_, err := e.store.EnsureCounterparty(e.as(), in)
		return err
	})
	// The exact path takes no lane lock, so capture may finish without waiting.
	waiters := 1
	if finished, err := e.waitForWaitersOrDone(t, held, 1, capture); finished {
		assertNoDeadlock(t, "capture", err)
		if err != nil {
			t.Fatalf("capture: %v", err)
		}
		capture, waiters = nil, 0
	}
	edit := inBackground(func() error {
		_, err := e.store.UpdateContact(e.as(), incumbent, UpdateContactInput{
			Emails: []ContactEmailInput{
				{Email: "ab@example.com", EmailType: "work", IsPrimary: true},
				{Email: "ab+y@example.com", EmailType: "work"},
			},
		})
		return err
	})
	e.waitForWaiters(t, held, waiters+1)
	held.finish(t)

	if capture != nil {
		err := <-capture
		assertNoDeadlock(t, "capture", err)
		if err != nil {
			t.Fatalf("capture: %v", err)
		}
	}
	err := <-edit
	assertNoDeadlock(t, "the edit", err)
	if err != nil {
		t.Fatalf("the edit: %v", err)
	}
}
