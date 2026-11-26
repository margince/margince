// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package unitsql_test

// Driven on a real Postgres as the application role, because the claim is
// about which role a statement runs as and no fake has one.

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/unitsql"
	"github.com/margince/margince/backend/internal/compose/unitsql/unitsqltest"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/pkg/extension"
)

const ownTable = "ext.ext_alpha_scratch"

// appConn connects as the application role to a migrated database, with the
// unit's own table in place. configure adjusts the connection, as a test needs.
func appConn(t *testing.T, configure func(*pgx.ConnConfig)) *pgx.Conn {
	t.Helper()
	ctx := context.Background()
	ownerDSN, appDSN := os.Getenv("MARGINCE_TEST_DSN"), os.Getenv("MARGINCE_TEST_APP_DSN")
	if ownerDSN == "" || appDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN / MARGINCE_TEST_APP_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := owner.Close(ctx); err != nil {
			t.Errorf("closing the owner connection: %v", err)
		}
	}()
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	unitsqltest.ScratchTable(t, ownTable)
	config, err := pgx.ParseConfig(appDSN)
	if err != nil {
		t.Fatal(err)
	}
	configure(config)
	app, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := app.Close(context.Background()); err != nil {
			t.Errorf("closing the app connection: %v", err)
		}
	})
	return app
}

// asConfigured leaves the connection as the DSN describes it.
func asConfigured(*pgx.ConnConfig) {}

// inTx runs fn through unitsql.Run in one transaction, and commits when Run
// returns nil, the way Runtime.Tx does.
func inTx(t *testing.T, app *pgx.Conn, fn func(ctx context.Context, sql unitsql.Tx, raw pgx.Tx) error) error {
	t.Helper()
	return pgx.BeginFunc(context.Background(), app, func(tx pgx.Tx) error {
		return unitsql.Run(tx, func(sql unitsql.Tx) error { return fn(context.Background(), sql, tx) })
	})
}

func rowCount(t *testing.T, app *pgx.Conn) int {
	t.Helper()
	var n int
	if err := app.QueryRow(context.Background(), `SELECT count(*) FROM `+ownTable).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAUnitReadsAndWritesItsOwnTable(t *testing.T) {
	app := appConn(t, asConfigured)
	err := inTx(t, app, func(ctx context.Context, sql unitsql.Tx, _ pgx.Tx) error {
		if _, err := sql.Exec(ctx, `INSERT INTO `+ownTable+` (id, body) VALUES (gen_random_uuid(), $1)`, "kept"); err != nil {
			return err
		}
		var body string
		if err := sql.QueryRow(ctx, `SELECT body FROM `+ownTable).Scan(&body); err != nil || body != "kept" {
			return errors.Join(err, errors.New("the row just written is not read back"))
		}
		rows, err := sql.Query(ctx, `SELECT body FROM `+ownTable)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("a unit's own table: %v", err)
	}
	if n := rowCount(t, app); n != 1 {
		t.Errorf("%d row(s) in the unit's own table, want 1", n)
	}
}

// Every statement here names a table outside schema ext. The last two build
// the name at run time, which the static gate on unit SQL cannot read.
func TestAStatementOnACoreTableIsRefused(t *testing.T) {
	app := appConn(t, asConfigured)
	for name, run := range map[string]func(ctx context.Context, sql unitsql.Tx) error{
		"Exec": func(ctx context.Context, sql unitsql.Tx) error {
			_, err := sql.Exec(ctx, `DELETE FROM contact`)
			return err
		},
		"Query": func(ctx context.Context, sql unitsql.Tx) error {
			rows, err := sql.Query(ctx, `SELECT id FROM contact`)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
			}
			return rows.Err()
		},
		"QueryRow": func(ctx context.Context, sql unitsql.Tx) error {
			var n int
			return sql.QueryRow(ctx, `SELECT count(*) FROM contact`).Scan(&n)
		},
		"INSERT": func(ctx context.Context, sql unitsql.Tx) error {
			_, err := sql.Exec(ctx, `INSERT INTO contact (id) VALUES (gen_random_uuid())`)
			return err
		},
		"extension_secret": func(ctx context.Context, sql unitsql.Tx) error {
			var n int
			return sql.QueryRow(ctx, `SELECT count(*) FROM extension_secret`).Scan(&n)
		},
		"name built in Go": func(ctx context.Context, sql unitsql.Tx) error {
			_, err := sql.Exec(ctx, "DELETE FROM "+strings.Join([]string{"con", "tact"}, ""))
			return err
		},
		"name built in SQL": func(ctx context.Context, sql unitsql.Tx) error {
			_, err := sql.Exec(ctx, `DO $$ BEGIN EXECUTE format('DELETE FROM %I', 'con' || 'tact'); END $$`)
			return err
		},
	} {
		err := inTx(t, app, func(ctx context.Context, sql unitsql.Tx, _ pgx.Tx) error { return run(ctx, sql) })
		if !errors.Is(err, extension.ErrCoreTableRefused) {
			t.Errorf("%s answered %v, want ErrCoreTableRefused", name, err)
		}
	}
}

// A string of statements could lift the role, write, and take the role back,
// so the check after the statement sees nothing. Each call takes one, even on
// a connection whose default is the simple protocol, which runs a string.
func TestAStringOfStatementsIsRefused(t *testing.T) {
	app := appConn(t, func(c *pgx.ConnConfig) { c.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol })
	escape := `RESET ROLE; INSERT INTO ` + ownTable + ` (id) VALUES (gen_random_uuid()); SET LOCAL ROLE ` + unitsql.Role
	if err := inTx(t, app, func(ctx context.Context, sql unitsql.Tx, _ pgx.Tx) error {
		_, err := sql.Exec(ctx, escape)
		return err
	}); err == nil {
		t.Error("a string of statements ran")
	}
	if n := rowCount(t, app); n != 0 {
		t.Errorf("%d row(s) written by the string of statements, want 0", n)
	}
}

// A statement that ends in another role is caught, and its transaction cannot
// commit, even when the unit swallows the error.
func TestAStatementThatLeavesTheRoleCannotCommit(t *testing.T) {
	app := appConn(t, asConfigured)
	err := inTx(t, app, func(ctx context.Context, sql unitsql.Tx, _ pgx.Tx) error {
		_, stmtErr := sql.Exec(ctx, `DO $$ BEGIN SET LOCAL ROLE margince_app; INSERT INTO `+ownTable+` (id) VALUES (gen_random_uuid()); END $$`)
		if !errors.Is(stmtErr, extension.ErrCoreTableRefused) {
			t.Errorf("a statement that changed the role answered %v, want ErrCoreTableRefused", stmtErr)
		}
		return nil
	})
	if err == nil {
		t.Error("the transaction committed after a statement left the unit role")
	}
	if n := rowCount(t, app); n != 0 {
		t.Errorf("%d row(s) left, want 0: the write must roll back", n)
	}
}

// The transaction is back on the application role after each verb. That holds
// on success, on an empty match, and on an error the client raises before sending.
func TestTheRoleIsLiftedAfterEveryStatement(t *testing.T) {
	app := appConn(t, asConfigured)
	err := inTx(t, app, func(ctx context.Context, sql unitsql.Tx, raw pgx.Tx) error {
		var body string
		for name, run := range map[string]func() error{
			"Exec": func() error { _, err := sql.Exec(ctx, `SELECT 1`); return err },
			"empty match": func() error {
				if err := sql.QueryRow(ctx, `SELECT body FROM `+ownTable).Scan(&body); !errors.Is(err, extension.ErrNoRows) {
					return errors.Join(err, errors.New("an empty match did not answer ErrNoRows"))
				}
				return nil
			},
			"client error": func() error {
				if _, err := sql.Exec(ctx, `SELECT $1::int`, make(chan int)); err == nil {
					return errors.New("an argument that cannot be encoded was accepted")
				}
				return nil
			},
			"Query": func() error {
				rows, err := sql.Query(ctx, `SELECT body FROM `+ownTable)
				if err != nil {
					return err
				}
				rows.Close()
				return rows.Err()
			},
		} {
			if err := run(); err != nil {
				return err
			}
			var role string
			if err := raw.QueryRow(ctx, `SELECT current_user::text`).Scan(&role); err != nil {
				return err
			}
			if role == unitsql.Role {
				t.Errorf("after %s the transaction still runs as %s", name, role)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// cancelAfter cancels the unit's context the moment one statement has run,
// before the check that follows it.
type cancelAfter struct {
	statement string
	cancel    context.CancelFunc
	hit       bool
}

func (c *cancelAfter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	c.hit = data.SQL == c.statement
	return ctx
}

func (c *cancelAfter) TraceQueryEnd(_ context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if c.hit {
		c.cancel()
	}
}

// A check that cannot run proves nothing, so the transaction must not commit,
// even when the unit swallows the error its statement answered.
func TestACancelledCheckRefusesTheCommit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	write := `INSERT INTO ` + ownTable + ` (id) VALUES (gen_random_uuid())`
	app := appConn(t, func(c *pgx.ConnConfig) { c.Tracer = &cancelAfter{statement: write, cancel: cancel} })
	err := inTx(t, app, func(_ context.Context, sql unitsql.Tx, _ pgx.Tx) error {
		if _, stmtErr := sql.Exec(ctx, write); stmtErr == nil {
			t.Error("the check after a cancelled context answered no error")
		}
		return nil
	})
	if err == nil {
		t.Error("the transaction committed although the check after its statement never ran")
	}
	if n := rowCount(t, app); n != 0 {
		t.Errorf("%d row(s) committed, want 0", n)
	}
}
