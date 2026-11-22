// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// A row that holds the retired country-named basis moves onto `consent`.
//
// The test puts the two basis checks back to the list they had before, writes
// rows under that list, then applies the real migration files. Seeding after
// the migration would pass for a migration that moves nothing.

import (
	"context"
	"testing"
)

const (
	basisListMigration      = "core/1795390725_the_basis_list_names_no_country"
	basisListCheckMigration = "core/1795390726_the_basis_list_check_is_validated"
	retiredBasis            = "vn_subject_agreement"
)

func TestARowHoldingTheRetiredBasisMovesOntoConsent(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	applyMigrationFile(t, conn, basisListCheckMigration+".down.sql")
	applyMigrationFile(t, conn, basisListMigration+".down.sql")

	var contact string
	if err := conn.QueryRow(ctx,
		`INSERT INTO contact (full_name, source, captured_by)
		 VALUES ('Basis Subject', 'manual', 'human:seed') RETURNING id`).Scan(&contact); err != nil {
		t.Fatalf("seeding the contact: %v", err)
	}
	if _, err := conn.Exec(ctx,
		`INSERT INTO communication_basis (contact_id, kind, captured_by) VALUES ($1, $2, 'test')`,
		contact, retiredBasis); err != nil {
		t.Fatalf("seeding a basis row under the old list: %v", err)
	}
	// The delivery a decision hangs off is not what is under test.
	if _, err := conn.Exec(ctx,
		`ALTER TABLE communication_decision DROP CONSTRAINT communication_decision_delivery_id_fkey`); err != nil {
		t.Fatalf("freeing the decision from its delivery: %v", err)
	}
	if _, err := conn.Exec(ctx,
		`INSERT INTO communication_decision
		   (delivery_id, decision_set_id, recipient_address, phase, resolved_category,
		    verdict, reason_code, basis, mode, actor)
		 VALUES (gen_random_uuid(), gen_random_uuid(), 'a@example.com', 'staging', 'reply_to_inbound',
		    'allow', 'test', $1, 'enforce', 'test')`, retiredBasis); err != nil {
		t.Fatalf("seeding a decision row under the old list: %v", err)
	}

	applyMigrationFile(t, conn, basisListMigration+".up.sql")
	applyMigrationFile(t, conn, basisListCheckMigration+".up.sql")

	for _, q := range []string{
		`SELECT kind FROM communication_basis WHERE contact_id = '` + contact + `'`,
		`SELECT basis FROM communication_decision WHERE recipient_address = 'a@example.com'`,
	} {
		var got string
		if err := conn.QueryRow(ctx, q).Scan(&got); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		if got != "consent" {
			t.Errorf("%s = %q, want consent: the agreement is what the retired value meant", q, got)
		}
	}

	var unvalidated int
	if err := conn.QueryRow(ctx,
		`SELECT count(*) FROM pg_constraint
		  WHERE conname IN ('communication_basis_kind', 'communication_decision_basis') AND NOT convalidated`).
		Scan(&unvalidated); err != nil {
		t.Fatalf("reading the constraint state: %v", err)
	}
	if unvalidated != 0 {
		t.Errorf("%d basis checks are still unvalidated after the validating migration", unvalidated)
	}
	if _, err := conn.Exec(ctx,
		`INSERT INTO communication_basis (contact_id, kind, captured_by) VALUES ($1, $2, 'test')`,
		contact, retiredBasis); err == nil {
		t.Error("the list still accepts the retired basis")
	}
}
