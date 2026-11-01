// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// A row that expires says it expires after it was created.
//
// Five of these tables hand out authority: a passport, an oauth grant, an
// authorization code, a deal-room session and a deal-room invitation are bearer
// credentials. A row whose expiry precedes its own creation has a lifetime
// nobody can reason about. A reader asking whether it is still valid gets an
// answer that depends on which column it trusted.
//
// Derived from the catalog rather than listed, so the fourteenth table to carry
// `expires_at` is asked the same question. The creation column is whichever of
// `created_at` and `issued_at` the table carries.

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// lifetimeFloor is the number of tables known to carry `expires_at`. A reading
// below it means the catalog query stopped seeing the class.
const lifetimeFloor = 23

// unboundedLifetime is the table whose expiry is written before its creation by
// design, with the reason beside it.
var unboundedLifetime = gatekit.Waive(map[string]string{
	"mfa_challenge_spent": "its expires_at is the CHALLENGE's expiry, copied in so the row can be reaped once the signature stops verifying rather than this row's own lifetime. The table keeps no creation stamp because it is a nonce ledger and that instant is the only thing it needs",
	"approval":            "superseding a pending approval writes expires_at = now() - interval '1 day' (approvals/staging.go), so the row reads as expired under the service clock as well as the database one, which may trail it by ordinary skew. An approval superseded an hour after it was raised expires a day before it was created, and bounding the column would refuse supersession",
})

func TestEveryExpiringRowExpiresAfterItWasCreated(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	rows, err := conn.Query(ctx, `
		SELECT c.relname,
		       max(CASE WHEN a.attname IN ('created_at', 'issued_at', 'fetched_at') THEN a.attname END),
		       (SELECT coalesce(array_agg(pg_get_constraintdef(k.oid)), '{}')
		          FROM pg_constraint k
		         WHERE k.conrelid = c.oid AND k.contype = 'c'
		           AND pg_get_constraintdef(k.oid) LIKE '%expires_at%')
		  FROM pg_class c
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		  JOIN pg_attribute a ON a.attrelid = c.oid AND NOT a.attisdropped
		 WHERE n.nspname = 'public' AND c.relkind = 'r'
		   AND EXISTS (SELECT 1 FROM pg_attribute e
		                WHERE e.attrelid = c.oid AND e.attname = 'expires_at'
		                  AND NOT e.attisdropped)
		 GROUP BY c.relname, c.oid
		 ORDER BY 1`)
	if err != nil {
		t.Fatalf("reading the expiring tables: %v", err)
	}
	type expiring struct {
		table, creation string
		// checks are this table's CHECK definitions that mention expires_at.
		// A table can carry several. assurance_resolution bounds the lifetime
		// and separately requires an expiry for two of its outcomes.
		//
		// So the bound is looked for as the comparison itself, never as a
		// mention of the column. Dropping it would otherwise leave the sibling
		// answering for it, with nothing failing.
		checks []string
	}
	found, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (expiring, error) {
		var e expiring
		var creation *string
		err := row.Scan(&e.table, &creation, &e.checks)
		if creation != nil {
			e.creation = *creation
		}
		return e, err
	})
	if err != nil {
		t.Fatalf("reading the expiring tables: %v", err)
	}
	if len(found) < lifetimeFloor {
		t.Fatalf("found %d tables carrying expires_at, fewer than the %d this schema has: "+
			"the query is reading a smaller schema than it thinks", len(found), lifetimeFloor)
	}

	for _, e := range found {
		if unboundedLifetime.Waived(t, e.table) {
			continue
		}
		if e.creation == "" {
			t.Errorf("%s carries expires_at and neither created_at nor issued_at, so its "+
				"lifetime cannot be bounded: give it a creation column or say here why it has none", e.table)
			continue
		}
		want := fmt.Sprintf("expires_at > %s", e.creation)
		if !slices.ContainsFunc(e.checks, func(def string) bool {
			return strings.Contains(def, want)
		}) {
			t.Errorf("%s carries expires_at and no CHECK compares it with %s: add "+
				"CHECK (%s)", e.table, e.creation, want)
		}
	}
	t.Logf("%d tables expire, %d waived", len(found), len(unboundedLifetime.Subjects()))
	unboundedLifetime.AssertAllMatched(t)
}
