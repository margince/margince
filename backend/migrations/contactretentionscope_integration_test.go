// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// A retention policy written for people keeps acting now that they are contacts.
//
// The rename moved the evaluator's selector to `contact/no_consent_no_deal`, so a
// row still scoped to `person` is skipped every night. The shipped migration is
// replayed over the two shapes an installation can be in: the stale row alone,
// which must carry over as it was set, and the stale row beside a contact rule an
// admin added since, which must leave the admin's rule standing.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

const contactRetentionMigration = "1791191324_a_contact_retention_policy_keeps_acting_after_the_rename.up.sql"

type retentionRow struct {
	objectType string
	days       int
	action     string
	basis      *string
}

func noConsentRows(t *testing.T, conn *pgx.Conn) []retentionRow {
	t.Helper()
	rows, err := conn.Query(context.Background(), `
		SELECT object_type, retain_days, action, lawful_basis FROM retention_policy
		 WHERE category = 'no_consent_no_deal' ORDER BY object_type`)
	if err != nil {
		t.Fatalf("reading the policies: %v", err)
	}
	got, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (retentionRow, error) {
		var row retentionRow
		return row, r.Scan(&row.objectType, &row.days, &row.action, &row.basis)
	})
	if err != nil {
		t.Fatalf("reading the policies: %v", err)
	}
	return got
}

func TestAPersonRetentionPolicyActsOnContactsAfterTheRename(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()
	reset := func() {
		if _, err := conn.Exec(ctx, `DELETE FROM retention_policy WHERE category = 'no_consent_no_deal'`); err != nil {
			t.Fatalf("clearing the policies: %v", err)
		}
	}
	t.Cleanup(reset)

	t.Run("the stale row carries over as it was set", func(t *testing.T) {
		reset()
		if _, err := conn.Exec(ctx, `
			INSERT INTO retention_policy (object_type, category, retain_days, action, lawful_basis)
			VALUES ('person', 'no_consent_no_deal', 730, 'anonymize', 'storage_limitation')`); err != nil {
			t.Fatalf("seeding the stale row: %v", err)
		}
		replayMigration(t, conn, contactRetentionMigration)
		got := noConsentRows(t, conn)
		if len(got) != 1 || got[0].objectType != "contact" || got[0].days != 730 || got[0].action != "anonymize" ||
			got[0].basis == nil || *got[0].basis != "storage_limitation" {
			t.Fatalf("after the migration: %+v, want one contact rule of 730 days, anonymize, storage_limitation", got)
		}
	})

	t.Run("a contact rule an admin added since is the one that stands", func(t *testing.T) {
		reset()
		if _, err := conn.Exec(ctx, `
			INSERT INTO retention_policy (object_type, category, retain_days, action, lawful_basis)
			VALUES ('person', 'no_consent_no_deal', 730, 'anonymize', 'storage_limitation'),
			       ('contact', 'no_consent_no_deal', 400, 'archive', 'legal_obligation')`); err != nil {
			t.Fatalf("seeding both rows: %v", err)
		}
		replayMigration(t, conn, contactRetentionMigration)
		got := noConsentRows(t, conn)
		if len(got) != 1 || got[0].objectType != "contact" || got[0].days != 400 || got[0].action != "archive" ||
			got[0].basis == nil || *got[0].basis != "legal_obligation" {
			t.Fatalf("after the migration: %+v, want only the admin's contact rule (400 days, archive, legal_obligation)", got)
		}
	})
}
