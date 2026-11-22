// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// Every foreign key says what a parent delete does to the child.
//
// Omitting the clause is not neutral: Postgres reads it as NO ACTION, so the row
// refuses the delete anyway — but nobody wrote that down, and the next reader cannot
// tell a considered refusal from a clause somebody forgot. The three the tree does
// write mean different things: CASCADE for a row the parent owns, SET NULL for an
// attribution the record outlives, RESTRICT for a catalog parent nothing may orphan.
//
// Postgres normalises an explicit ON DELETE NO ACTION away, so a considered one and a
// forgotten clause leave the same catalog: confdeltype reads 'a' for both, and so does
// pg_get_constraintdef — verified by writing one and watching this gate still report
// the key.
//
// So the rule is a NON-DEFAULT clause. A key that genuinely means "refuse the delete"
// writes RESTRICT, which is recorded, and which also checks the parent row
// immediately rather than deferring to the end of the statement.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

// unwrittenDeleteRule finds the foreign keys left on the default: NO ACTION, which is
// what Postgres records both for an omitted clause and for an explicit one.
const unwrittenDeleteRule = `
SELECT c.conrelid::regclass::text || ' (' ||
       (SELECT string_agg(att.attname, ', ' ORDER BY u.ord)
          FROM unnest(c.conkey) WITH ORDINALITY u(k, ord)
          JOIN pg_attribute att ON att.attrelid = c.conrelid AND att.attnum = u.k)
       || ') -> ' || c.confrelid::regclass::text
  FROM pg_constraint c
  JOIN pg_class t ON t.oid = c.conrelid
  JOIN pg_namespace n ON n.oid = t.relnamespace
 WHERE c.contype = 'f' AND n.nspname = 'public'
   AND c.confdeltype = 'a'
 ORDER BY 1`

// foreignKeyCount tallies the schema's foreign keys, answered or not. It is the floor
// that stops this passing by reading nothing.
const foreignKeyCount = `
SELECT count(*) FROM pg_constraint c
  JOIN pg_class t ON t.oid = c.conrelid
  JOIN pg_namespace n ON n.oid = t.relnamespace
 WHERE c.contype = 'f' AND n.nspname = 'public'`

// TestEveryForeignKeyStatesItsDeleteRule fails on a key with no stated delete rule.
//
// A row the parent owns CASCADEs. An attribution the record outlives is SET NULL.
// A catalog or versioned parent is RESTRICT. So is a NOT NULL reference to app_user:
// a user is deactivated and handed over, never deleted.
func TestEveryForeignKeyStatesItsDeleteRule(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	var keys int
	if err := conn.QueryRow(ctx, foreignKeyCount).Scan(&keys); err != nil {
		t.Fatalf("counting foreign keys: %v", err)
	}
	// A collapse means the query stopped recognising keys. Every key would then
	// read as answered.
	if keys < 400 {
		t.Fatalf("this gate found %d foreign key(s) and expects at least 400 — it has stopped "+
			"recognising them rather than the schema having given them up", keys)
	}

	rows, err := conn.Query(ctx, unwrittenDeleteRule)
	if err != nil {
		t.Fatalf("reading the keys that state no delete rule: %v", err)
	}
	unwritten, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("reading the keys that state no delete rule: %v", err)
	}
	for _, key := range unwritten {
		t.Errorf("%s leaves a parent delete to the default. Say what the delete does — CASCADE "+
			"for a row the parent owns, SET NULL for an attribution the record outlives, RESTRICT "+
			"for a catalog parent nothing may orphan. Writing NO ACTION is not available: Postgres "+
			"normalises it away, so it reads here exactly like the omission.", key)
	}
}
