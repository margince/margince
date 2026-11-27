// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// The retired country-named basis leaves the list without rewriting evidence.
//
// Each test puts the two basis checks back to the list they had before, then
// applies the real migration files. Seeding after the migration would pass for
// a migration that mishandles the rows it finds.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

const (
	basisListMigration      = "core/1795388887_the_basis_vocabulary_names_no_country"
	basisListCheckMigration = "core/1795388888_the_basis_vocabulary_names_no_country_validated"
	retiredBasis            = "vn_subject_agreement"
)

func asTheBasisListWasBefore(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	applyMigrationFile(t, conn, basisListCheckMigration+".down.sql")
	applyMigrationFile(t, conn, basisListMigration+".down.sql")
}

func seedContactWithRetiredBasis(t *testing.T, conn *pgx.Conn) string {
	t.Helper()
	ctx := context.Background()
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
	return contact
}

func basisConstraintCount(t *testing.T, conn *pgx.Conn, validated bool) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(context.Background(),
		`SELECT count(*) FROM pg_constraint
		  WHERE (conrelid, conname) IN (
		          ('communication_basis'::regclass, 'communication_basis_kind'),
		          ('communication_decision'::regclass, 'communication_decision_basis'))
		    AND convalidated = $1`, validated).Scan(&n); err != nil {
		t.Fatalf("reading the constraint state: %v", err)
	}
	return n
}

func TestAnOldBasisRowStaysAsItWasAndTheListStaysClosed(t *testing.T) {
	t.Parallel()
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()
	asTheBasisListWasBefore(t, conn)
	contact := seedContactWithRetiredBasis(t, conn)

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
		if got != retiredBasis {
			t.Errorf("%s = %q, want %q: a row is never relabelled as another basis", q, got, retiredBasis)
		}
	}
	if n := basisConstraintCount(t, conn, true); n != 2 {
		t.Errorf("%d basis checks are validated, want 2: a table holding old rows keeps a check", n)
	}
	if n := basisConstraintCount(t, conn, false); n != 0 {
		t.Errorf("%d basis checks are unvalidated, which would block updates of the old rows", n)
	}
	if _, err := conn.Exec(ctx,
		`UPDATE communication_basis SET revoked_at = now() WHERE contact_id = $1`, contact); err != nil {
		t.Errorf("a row holding the old value can no longer change: %v", err)
	}
	if _, err := conn.Exec(ctx,
		`INSERT INTO communication_basis (contact_id, kind, captured_by) VALUES ($1, 'not_a_basis', 'test')`,
		contact); err == nil {
		t.Error("a table holding old rows accepts an arbitrary basis string")
	}
	if _, err := conn.Exec(ctx,
		`INSERT INTO communication_decision
		   (delivery_id, decision_set_id, recipient_address, phase, resolved_category,
		    verdict, reason_code, basis, mode, actor)
		 VALUES (gen_random_uuid(), gen_random_uuid(), 'b@example.com', 'staging', 'reply_to_inbound',
		    'allow', 'test', 'not_a_basis', 'enforce', 'test')`); err == nil {
		t.Error("a decision table holding old rows accepts an arbitrary basis string")
	}
}

func TestACleanTableRefusesTheRetiredBasisInTheDatabase(t *testing.T) {
	t.Parallel()
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()
	asTheBasisListWasBefore(t, conn)
	// A check of the same name on another table is not the migration's to settle.
	if _, err := conn.Exec(ctx,
		`CREATE TABLE basis_decoy (kind text)`); err != nil {
		t.Fatalf("creating the decoy table: %v", err)
	}
	// Added after the table exists: CREATE TABLE records every check as validated.
	if _, err := conn.Exec(ctx,
		`ALTER TABLE basis_decoy ADD CONSTRAINT communication_basis_kind CHECK (kind <> 'x') NOT VALID`); err != nil {
		t.Fatalf("adding the unvalidated decoy check: %v", err)
	}

	applyMigrationFile(t, conn, basisListMigration+".up.sql")
	applyMigrationFile(t, conn, basisListCheckMigration+".up.sql")

	var decoyValidated bool
	if err := conn.QueryRow(ctx,
		`SELECT convalidated FROM pg_constraint WHERE conrelid = 'basis_decoy'::regclass`).Scan(&decoyValidated); err != nil {
		t.Fatalf("reading the decoy check: %v", err)
	}
	if decoyValidated {
		t.Error("the migration validated a check of the same name on another table")
	}
	if n := basisConstraintCount(t, conn, false); n != 0 {
		t.Errorf("%d basis checks are still unvalidated on a table with no old rows", n)
	}
	if n := basisConstraintCount(t, conn, true); n != 2 {
		t.Errorf("%d basis checks are validated, want 2", n)
	}
	var contact string
	if err := conn.QueryRow(ctx,
		`INSERT INTO contact (full_name, source, captured_by)
		 VALUES ('Clean Subject', 'manual', 'human:seed') RETURNING id`).Scan(&contact); err != nil {
		t.Fatalf("seeding the contact: %v", err)
	}
	if _, err := conn.Exec(ctx,
		`INSERT INTO communication_basis (contact_id, kind, captured_by) VALUES ($1, $2, 'test')`,
		contact, retiredBasis); err == nil {
		t.Error("the list still accepts the retired basis")
	}
}
