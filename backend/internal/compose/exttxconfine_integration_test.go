// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A unit's SQL reaches its own tables and nothing else, enforced by the
// database. Driven through callRuntime.Tx on a real Postgres, because the claim
// is about which role a statement runs as and no fake has one.

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/pkg/extension"
)

const confineTable = "ext.ext_notes_scratch"

// unitScratchTable creates (id uuid, body text) the way a unit's migration
// leaves a table. It sits in schema ext, owned by the migrate role, and is
// granted to the runtime role. The core migration's default privileges reach
// the unit role, as they do for a real unit.
func unitScratchTable(t *testing.T, table string) {
	t.Helper()
	ctx := context.Background()
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	if ownerDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(), `DROP TABLE IF EXISTS `+table); err != nil {
			t.Errorf("dropping the unit's own table: %v", err)
		}
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing the owner connection: %v", err)
		}
	})
	for _, stmt := range []string{
		`DROP TABLE IF EXISTS ` + table,
		`CREATE TABLE ` + table + ` (id uuid PRIMARY KEY, body text)`,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON ` + table + ` TO margince_app`,
	} {
		if _, err := owner.Exec(ctx, stmt); err != nil {
			t.Fatalf("preparing the unit's own table: %v", err)
		}
	}
}

func (e coreEnv) inTx(t *testing.T, fn func(ctx context.Context, tx extension.Tx) error) error {
	t.Helper()
	rt, ctx, cancel := e.runtime(t)
	defer cancel()
	return rt.Tx(ctx, fn)
}

func TestAUnitsSQLOnACoreTableIsRefused(t *testing.T) {
	e := setupCore(t)

	for name, run := range map[string]func(ctx context.Context, tx extension.Tx) error{
		"Exec": func(ctx context.Context, tx extension.Tx) error {
			_, err := tx.Exec(ctx, `DELETE FROM contact`)
			return err
		},
		"Query": func(ctx context.Context, tx extension.Tx) error {
			rows, err := tx.Query(ctx, `SELECT id FROM contact`)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
			}
			return rows.Err()
		},
		"QueryRow": func(ctx context.Context, tx extension.Tx) error {
			var n int
			return tx.QueryRow(ctx, `SELECT count(*) FROM contact`).Scan(&n)
		},
	} {
		if err := e.inTx(t, run); !errors.Is(err, extension.ErrCoreTableRefused) {
			t.Errorf("%s on a core table answered %v, want ErrCoreTableRefused", name, err)
		}
	}
	if n := e.count(t, `SELECT count(*) FROM contact`); n != 1 {
		t.Errorf("%d contact row(s) left, want the 1 seeded: the refused DELETE must not land", n)
	}
}

// A second statement in the same string would run after a RESET ROLE the unit
// put first, so the seam takes one statement per call.
func TestAUnitCannotLiftTheRoleWithAStringOfStatements(t *testing.T) {
	e := setupCore(t)
	escape := `RESET ROLE; DELETE FROM contact`

	for name, run := range map[string]func(ctx context.Context, tx extension.Tx) error{
		"Exec": func(ctx context.Context, tx extension.Tx) error {
			_, err := tx.Exec(ctx, escape)
			return err
		},
		"Query": func(ctx context.Context, tx extension.Tx) error {
			rows, err := tx.Query(ctx, escape)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
			}
			return rows.Err()
		},
		"QueryRow": func(ctx context.Context, tx extension.Tx) error {
			var n int
			return tx.QueryRow(ctx, escape).Scan(&n)
		},
	} {
		if err := e.inTx(t, run); err == nil {
			t.Errorf("%s ran a string of statements, want it refused", name)
		}
	}
	if n := e.count(t, `SELECT count(*) FROM contact`); n != 1 {
		t.Errorf("%d contact row(s) left, want the 1 seeded: the escape must not land", n)
	}
}

// A statement that leaves the unit role behind is caught when it ends, and its
// transaction cannot commit, even when the unit swallows the error.
func TestAStatementThatLeavesTheUnitRoleCannotCommit(t *testing.T) {
	e := setupCore(t)

	err := e.inTx(t, func(ctx context.Context, tx extension.Tx) error {
		_, stmtErr := tx.Exec(ctx, `DO $$ BEGIN SET LOCAL ROLE margince_app; DELETE FROM contact; END $$`)
		if !errors.Is(stmtErr, extension.ErrCoreTableRefused) {
			t.Errorf("a statement that changed the role answered %v, want ErrCoreTableRefused", stmtErr)
		}
		return nil
	})
	if err == nil {
		t.Error("the transaction committed after a statement left the unit role")
	}
	if n := e.count(t, `SELECT count(*) FROM contact`); n != 1 {
		t.Errorf("%d contact row(s) left, want the 1 seeded: the delete must roll back", n)
	}
}

// An error the client raises before anything is sent leaves the transaction
// usable, so the core door after it must still run as the core.
func TestTheCoreDoorWorksAfterAClientSideSQLError(t *testing.T) {
	e := setupCore(t)

	err := e.inTx(t, func(ctx context.Context, tx extension.Tx) error {
		if _, stmtErr := tx.Exec(ctx, `SELECT $1::int`, make(chan int)); stmtErr == nil {
			t.Error("an argument that cannot be encoded was accepted")
		}
		_, coreErr := tx.Core().Activities().Create(ctx, e.aNote(e.subject))
		return coreErr
	})
	if err != nil {
		t.Fatalf("the core door after a client-side error: %v", err)
	}
}

func TestAUnitsSQLOnItsOwnTableStillWorks(t *testing.T) {
	e := setupCore(t)
	unitScratchTable(t, confineTable)

	err := e.inTx(t, func(ctx context.Context, tx extension.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO `+confineTable+` (id, body) VALUES (gen_random_uuid(), 'kept')`); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT body FROM `+confineTable)
		if err != nil {
			return err
		}
		defer rows.Close()
		if !rows.Next() {
			return errors.New("the unit's own row is not readable")
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("a unit's own table: %v", err)
	}
	if n := e.count(t, `SELECT count(*) FROM `+confineTable); n != 1 {
		t.Errorf("%d row(s) in the unit's own table, want 1", n)
	}
}

// The role covers the unit's SQL alone. The core door still writes records
// when used between the unit's own statements in one transaction.
func TestTheCoreDoorKeepsItsAuthorityAroundTheUnitsSQL(t *testing.T) {
	e := setupCore(t)
	unitScratchTable(t, confineTable)

	err := e.inTx(t, func(ctx context.Context, tx extension.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO `+confineTable+` (id, body) VALUES (gen_random_uuid(), 'x')`); err != nil {
			return err
		}
		if _, err := tx.Core().Activities().Create(ctx, e.aNote(e.subject)); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM `+confineTable).Scan(&n); err != nil {
			return err
		}
		_, err := tx.Core().Activities().Create(ctx, e.aNote(e.subject))
		return err
	})
	if err != nil {
		t.Fatalf("the core door around the unit's SQL: %v", err)
	}
	if n := e.count(t, `SELECT count(*) FROM activity`); n != 2 {
		t.Errorf("%d activities, want the 2 the door filed", n)
	}
}
