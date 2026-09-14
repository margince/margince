// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package database

// What the pool's two ceilings are worth against a real server. A runtime
// parameter that never reaches Postgres reads exactly like one that did: the
// config carries the value, SHOW would answer it, and the statement still runs
// forever. So these prove the KILL, not the setting.

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// queryCanceled is the SQLSTATE Postgres ends a statement with when
// statement_timeout fires.
const queryCanceled = "57014"

// rowQuerier is the one method these assertions need, so a pool and a
// transaction can each be asked what IT runs under.
type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// effectiveMilliseconds reads a duration setting in the unit Postgres stores
// it in. pg_settings rather than SHOW, because SHOW renders the value for a
// human — "1min", "20s" — and a test comparing those strings is asserting
// Postgres's formatting rather than the ceiling.
func effectiveMilliseconds(ctx context.Context, q rowQuerier, setting string) (int64, error) {
	var raw string
	if err := q.QueryRow(ctx,
		`SELECT setting FROM pg_settings WHERE name = $1`, setting).Scan(&raw); err != nil {
		return 0, err
	}
	return strconv.ParseInt(raw, 10, 64)
}

func ceilingTestDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("MARGINCE_TEST_DSN")
	if dsn == "" {
		t.Fatal("MARGINCE_TEST_DSN is not set — run `make db-up` and try again (integration tests fail loudly, they never skip)")
	}
	// Two connections, because each assertion below needs one and the lane's
	// connection budget is shared with every package running beside this one.
	return WithDSNParam(dsn, "pool_max_conns", "2")
}

// TestThePoolsCeilingsReachTheServer reads back what the connection actually
// runs under. The ceilings are milliseconds on the wire and durations in Go,
// and a factor of a thousand either way is a ceiling that never fires or one
// that fires on everything.
func TestThePoolsCeilingsReachTheServer(t *testing.T) {
	ctx := context.Background()
	pool, err := NewPool(ctx, ceilingTestDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	for setting, want := range map[string]time.Duration{
		"statement_timeout":                   StatementCeiling,
		"idle_in_transaction_session_timeout": IdleTransactionCeiling,
	} {
		got, err := effectiveMilliseconds(ctx, pool, setting)
		if err != nil {
			t.Fatalf("reading %s: %v", setting, err)
		}
		if got != want.Milliseconds() {
			t.Errorf("a connection runs under %s = %dms, want %dms", setting, got, want.Milliseconds())
		}
	}
}

// TestAStatementPastTheCeilingIsEnded is the property the ceiling exists for.
// It runs against a pool whose ceiling the DSN shortened, because the point is
// that a ceiling named this way ENDS a statement — proving it at the product's
// own thirty seconds would cost thirty seconds to learn the same thing.
func TestAStatementPastTheCeilingIsEnded(t *testing.T) {
	ctx := context.Background()
	pool, err := NewPool(ctx, WithDSNParam(ceilingTestDSN(t), "statement_timeout", "100"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	_, err = pool.Exec(ctx, `SELECT pg_sleep(5)`)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != queryCanceled {
		t.Fatalf("a statement past the ceiling ended with %v, want SQLSTATE %s — nothing else stops a query that has stopped making progress from holding its connection",
			err, queryCanceled)
	}
}

// TestATransactionsOwnBudgetOutranksThePoolsCeiling: the pool's ceiling is a
// floor under queries nobody thought about, never a cap on the ones somebody
// did. Every path that needs longer — the schema-changing roles, a background
// sweep — depends on being able to say so and be believed.
func TestATransactionsOwnBudgetOutranksThePoolsCeiling(t *testing.T) {
	ctx := context.Background()
	pool, err := NewPool(ctx, WithDSNParam(ceilingTestDSN(t), "statement_timeout", "100"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Reported rather than discarded: nothing here commits, so the rollback is
	// how this transaction is meant to end and a refusal means something else
	// already ended it — which would make the assertions below true of a
	// transaction the test no longer holds.
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rolling back the probe transaction: %v", err)
		}
	}()

	if err := BoundStatement(ctx, tx, 20*time.Second); err != nil {
		t.Fatalf("widening the budget: %v", err)
	}
	got, err := effectiveMilliseconds(ctx, tx, "statement_timeout")
	if err != nil {
		t.Fatalf("reading the widened budget: %v", err)
	}
	if want := (20 * time.Second).Milliseconds(); got != want {
		t.Fatalf("the transaction runs under statement_timeout = %dms, want %dms — the pool's ceiling outranked the budget the caller asked for", got, want)
	}
	// And it is the SERVER that agrees, not just the setting: a statement past
	// the pool's own 100ms completes because the transaction said it may.
	if _, err := tx.Exec(ctx, `SELECT pg_sleep(0.3)`); err != nil {
		t.Fatalf("a statement inside the widened budget was ended anyway: %v", err)
	}
}
