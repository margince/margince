// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// No foreign key rewrites the row that points at it.
//
// ON UPDATE CASCADE, SET NULL and SET DEFAULT all have the DATABASE write a
// child row when a parent key changes. No application code runs, so there is
// no audit_log row and no event_outbox row, and the write shape AGENTS.md
// states is bypassed. A row trigger still fires, so a version column moves and
// every client holding the old version gets a 409 nothing explains.
//
// Read off `pg_constraint` rather than the catalog file. There
// `pg_get_constraintdef` prints nothing for the default, so a key stating
// `NO ACTION` and a key saying nothing read alike in text. The column tells
// them apart: `confupdtype` is `a` for `NO ACTION` and `r` for `RESTRICT`,
// and those two are the only ones that refuse.
//
// The floor is the whole corpus of foreign keys, not the findings, so a query
// that stopped seeing them fails instead of passing.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// updateActionFloor is the count of foreign keys this schema is known to carry.
// A reading below it means the census stopped seeing the corpus.
const updateActionFloor = 400

// rewriteAllowed is the key whose update action is the behaviour, with the
// reason beside it. A waiver carries a sentence about its own key. That lets
// the next reader tell a decision from an oversight.
var rewriteAllowed = gatekit.Waive(map[string]string{
	"provider_run_category_run_fkey": "the composite key (run_id, contact_id, provider) copies two of the run's own columns so a unique index can hold one claim per category. A contact merge rewrites provider_run.contact_id (contacts/mergerelink.go), and the cascade is what carries the copies with it. RESTRICT here would refuse the merge instead",
})

// rewritingKey is one foreign key whose update action writes the child.
type rewritingKey struct {
	table, name, action string
}

func TestNoForeignKeyRewritesTheRowThatPointsAtIt(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	var total int
	if err := conn.QueryRow(ctx, `
		SELECT count(*) FROM pg_constraint c
		  JOIN pg_class t ON t.oid = c.conrelid
		  JOIN pg_namespace n ON n.oid = t.relnamespace
		 WHERE c.contype = 'f' AND n.nspname IN ('public', 'ext')`).Scan(&total); err != nil {
		t.Fatalf("counting the foreign keys: %v", err)
	}
	if total < updateActionFloor {
		t.Fatalf("counted %d foreign keys, fewer than the %d this schema carries: the census is reading a smaller schema than it thinks",
			total, updateActionFloor)
	}

	rows, err := conn.Query(ctx, `
		SELECT t.relname, c.conname, c.confupdtype FROM pg_constraint c
		  JOIN pg_class t ON t.oid = c.conrelid
		  JOIN pg_namespace n ON n.oid = t.relnamespace
		 WHERE c.contype = 'f' AND n.nspname IN ('public', 'ext')
		   AND c.confupdtype NOT IN ('a', 'r')
		 ORDER BY 1, 2`)
	if err != nil {
		t.Fatalf("reading the update actions: %v", err)
	}
	found, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (rewritingKey, error) {
		var k rewritingKey
		err := row.Scan(&k.table, &k.name, &k.action)
		return k, err
	})
	if err != nil {
		t.Fatalf("reading the update actions: %v", err)
	}

	for _, k := range found {
		if rewriteAllowed.Waived(t, k.name) {
			continue
		}
		t.Errorf("%s.%s carries ON UPDATE %s, so a parent key change has the database rewrite the child: no audit row, no event, and a version bump nothing explains. Use NO ACTION or RESTRICT, and rename through a migration instead",
			k.table, k.name, updateActionName(k.action))
	}
	t.Logf("read %d foreign keys, %d of which rewrite their child", total, len(found))
	// A waiver for a key that no longer rewrites anything is a line nobody
	// removes later. This fails on one instead, while it is still cheap.
	rewriteAllowed.AssertAllMatched(t)
}

// updateActionName spells pg_constraint.confupdtype the way the schema does.
func updateActionName(code string) string {
	switch code {
	case "c":
		return "CASCADE"
	case "n":
		return "SET NULL"
	case "d":
		return "SET DEFAULT"
	}
	return code
}
