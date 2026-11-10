// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"strings"
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
	var parent *pgx.Conn = first
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
	running := false
	for deadline := time.Now().Add(5 * time.Second); !running; {
		if time.Now().After(deadline) {
			t.Fatal("the sleeping query never started")
		}
		if err := watcher.QueryRow(context.Background(), `SELECT EXISTS (SELECT 1 FROM pg_stat_activity
			WHERE pid = $1 AND state = 'active' AND query LIKE '%pg_sleep%')`, pid).Scan(&running); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started := time.Now()
	_, err := shared.Exec(ctx, `SELECT 1`)
	if err == nil || !strings.Contains(err.Error(), "conn busy") {
		t.Errorf("a second goroutine on the connection answered %v, want conn busy", err)
	}
	if waited := time.Since(started); waited > time.Second {
		t.Errorf("the refusal took %s; it waited for the other query instead", waited)
	}
	if err := <-sleeping; err != nil {
		t.Errorf("the first query: %v", err)
	}
}
