// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package integration

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
)

// OwnerConn is the schema-owner connection tests use to shift timestamps the
// app role's workspace-bound path could never touch. A test gets one, however
// often it asks. It is shared only while idle; one closed, or left in a
// transaction or a query, is not, and the caller gets a fresh one. A test that
// needs a second connection, to hold a lock or race the first, asks
// FreshOwnerConn.
func OwnerConn(t *testing.T) *pgx.Conn {
	t.Helper()
	if held, ok := ownerConns.Load(t); ok {
		if conn, ok := held.(*pgx.Conn); ok && ownerConnIdle(conn) {
			return conn
		}
		return FreshOwnerConn(t)
	}
	conn := FreshOwnerConn(t)
	ownerConns.Store(t, conn)
	t.Cleanup(func() { ownerConns.Delete(t) })
	return conn
}

// ownerConns holds each running test's shared owner connection, keyed by its *testing.T.
var ownerConns sync.Map

// ownerConnIdle reports whether conn can be handed to the next caller: open,
// not in the middle of a query, and outside any transaction.
func ownerConnIdle(conn *pgx.Conn) bool {
	pg := conn.PgConn()
	return !conn.IsClosed() && !pg.IsBusy() && pg.TxStatus() == 'I'
}

// FreshOwnerConn opens a schema-owner connection of the test's own, closed when
// the test ends. It is never the one OwnerConn shares.
func FreshOwnerConn(t *testing.T) *pgx.Conn {
	t.Helper()
	dsn := os.Getenv("MARGINCE_TEST_DSN")
	if dsn == "" {
		t.Fatal("MARGINCE_TEST_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if conn.IsClosed() {
			return
		}
		if err := conn.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	return conn
}
