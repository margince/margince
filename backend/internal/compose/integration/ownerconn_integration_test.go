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

// A second query on a connection still running one is refused at once, never
// left waiting behind it. One goroutine holds the open rows: two goroutines on
// one pgx.Conn are a data race inside pgx, which -race reports.
func TestASecondQueryOnTheBusySharedOwnerConnectionFailsLoudly(t *testing.T) {
	shared := OwnerConn(t)
	rows, err := shared.Query(context.Background(), `SELECT pg_sleep(1)`)
	if err != nil {
		t.Fatalf("starting the long query: %v", err)
	}
	defer rows.Close()
	// Under a second, so a connection that queued the second query behind the
	// first answers with the deadline instead of conn busy.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := shared.Exec(ctx, `SELECT 1`); err == nil || !strings.Contains(err.Error(), "conn busy") {
		t.Errorf("a second query on the connection answered %v, want conn busy", err)
	}
}

// Goroutines asking at once still share the one connection.
func TestGoroutinesAskingAtOnceShareOneOwnerConnection(t *testing.T) {
	const askers = 8
	got := make([]*pgx.Conn, askers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range askers {
		wg.Go(func() {
			<-start
			got[i] = OwnerConn(t)
		})
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
	rows, err := shared.Query(context.Background(), `SELECT pg_sleep(1)`)
	if err != nil {
		t.Fatalf("starting the long query: %v", err)
	}
	defer rows.Close()
	other := OwnerConn(t)
	if other == shared {
		t.Fatal("the connection running a query was handed out again")
	}
	backendPID(t, other)
}
