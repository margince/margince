// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func backendPID(t *testing.T, conn *pgx.Conn) int {
	t.Helper()
	var pid int
	if err := conn.QueryRow(context.Background(), `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatalf("reading the backend: %v", err)
	}
	return pid
}

func TestATestSharesOneOwnerConnection(t *testing.T) {
	first, second := OwnerConn(t), OwnerConn(t)
	if first != second || backendPID(t, first) != backendPID(t, second) {
		t.Fatal("two calls in one test opened two owner connections")
	}
	if FreshOwnerConn(t) == first {
		t.Fatal("FreshOwnerConn handed out the shared connection")
	}
	parent := first
	t.Run("a subtest", func(t *testing.T) {
		if OwnerConn(t) == parent {
			t.Fatal("a subtest was handed its parent's connection")
		}
	})
}

// A connection inside a transaction would pull the next caller into it.
func TestTheSharedOwnerConnectionIsNotHandedOutInsideATransaction(t *testing.T) {
	ctx := context.Background()
	shared := OwnerConn(t)
	tx, err := shared.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rolling back: %v", err)
		}
	}()
	other := OwnerConn(t)
	if other == shared || backendPID(t, other) == backendPID(t, shared) {
		t.Fatal("the connection holding a transaction was handed out again")
	}
}

func TestAClosedSharedOwnerConnectionIsReplaced(t *testing.T) {
	shared := OwnerConn(t)
	if err := shared.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	replaced := OwnerConn(t)
	if replaced == shared {
		t.Fatal("a closed connection was handed out")
	}
	backendPID(t, replaced)
}

// A second goroutine on a connection already running a query is refused at
// once, never left waiting behind it.
func TestTwoGoroutinesOnTheSharedOwnerConnectionFailLoudly(t *testing.T) {
	shared := OwnerConn(t)
	pid := backendPID(t, shared)
	sleeping := make(chan error, 1)
	go func() {
		_, err := shared.Exec(context.Background(), `SELECT pg_sleep(1)`)
		sleeping <- err
	}()
	// Watched from a connection of its own, so the shared one has one user.
	watcher := FreshOwnerConn(t)
	waiting, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	for running := false; !running; {
		if err := watcher.QueryRow(waiting, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity
			WHERE pid = $1 AND state = 'active' AND query LIKE '%pg_sleep%')`, pid).Scan(&running); err != nil {
			t.Fatalf("the sleeping query never started: %v", err)
		}
	}
	// Under a second, so a connection that queued the second query behind the
	// first answers with the deadline instead of conn busy.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := shared.Exec(ctx, `SELECT 1`); err == nil || !strings.Contains(err.Error(), "conn busy") {
		t.Errorf("a second goroutine on the connection answered %v, want conn busy", err)
	}
	if err := <-sleeping; err != nil {
		t.Errorf("the first query: %v", err)
	}
}

// Goroutines asking at once still share the one connection.
func TestGoroutinesAskingAtOnceShareOneOwnerConnection(t *testing.T) {
	const askers = 8
	got := make([]*pgx.Conn, askers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range askers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			got[i] = OwnerConn(t)
		}()
	}
	close(start)
	wg.Wait()
	for i, conn := range got {
		if conn != got[0] {
			t.Fatalf("asker %d got its own connection while the others shared one", i)
		}
	}
}

// A connection running a query is not handed out on: the caller gets its own.
func TestTheSharedOwnerConnectionIsNotHandedOutWhileItRunsAQuery(t *testing.T) {
	shared := OwnerConn(t)
	pid := backendPID(t, shared)
	sleeping := make(chan error, 1)
	go func() {
		_, err := shared.Exec(context.Background(), `SELECT pg_sleep(1)`)
		sleeping <- err
	}()
	watcher := FreshOwnerConn(t)
	waiting, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	for running := false; !running; {
		if err := watcher.QueryRow(waiting, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity
			WHERE pid = $1 AND state = 'active' AND query LIKE '%pg_sleep%')`, pid).Scan(&running); err != nil {
			t.Fatalf("the sleeping query never started: %v", err)
		}
	}
	other := OwnerConn(t)
	if other == shared {
		t.Error("the connection running a query was handed out again")
	} else {
		backendPID(t, other)
	}
	if err := <-sleeping; err != nil {
		t.Errorf("the first query: %v", err)
	}
}
