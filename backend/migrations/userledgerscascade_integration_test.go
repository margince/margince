// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"
)

// A member's own run ledgers go with the member. Their readers scan the user id as
// non-null. Nothing sweeps a row that names nobody. So deleting the member must
// succeed and take those rows along, never refuse or orphan them.
func TestDeletingAMemberTakesTheirRunLedgersWithThem(t *testing.T) {
	t.Parallel()
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("opening the fixture transaction: %v", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rolling the fixture back: %v", err)
		}
	}()

	var member string
	if err := tx.QueryRow(ctx,
		`INSERT INTO app_user (email, display_name) VALUES ('ledger-member@example.test', 'Ledger Member') RETURNING id::text`).
		Scan(&member); err != nil {
		t.Fatalf("creating the member: %v", err)
	}
	for _, insert := range []string{
		`INSERT INTO brief_run (user_id, as_of, candidate_count, local_day, revenue_norm_minor)
		 VALUES ($1, now(), 0, current_date, 1)`,
		`INSERT INTO notification_digest_run (user_id, digest_date) VALUES ($1, current_date)`,
		`INSERT INTO report_run (asked_by, captured_by, group_floor, query) VALUES ($1, 'human:test', 0, '{}')`,
		`INSERT INTO worklist_snapshot (reader_id, as_of, buckets, expires_at, params_fingerprint, rows)
		 VALUES ($1, now(), '{}', now() + interval '1 hour', 'fingerprint', '[]')`,
	} {
		if _, err := tx.Exec(ctx, insert, member); err != nil {
			t.Fatalf("writing a ledger row for the member (%.40s…): %v", insert, err)
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM app_user WHERE id = $1`, member); err != nil {
		t.Fatalf("deleting the member was refused: %v", err)
	}
	for _, table := range []string{"brief_run", "notification_digest_run", "report_run", "worklist_snapshot"} {
		column := map[string]string{"report_run": "asked_by", "worklist_snapshot": "reader_id"}[table]
		if column == "" {
			column = "user_id"
		}
		var left int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE `+column+` = $1`, member).Scan(&left); err != nil {
			t.Fatalf("counting what %s kept: %v", table, err)
		}
		if left != 0 {
			t.Errorf("%s kept %d row(s) for the deleted member", table, left)
		}
	}
}
