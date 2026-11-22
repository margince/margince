// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	targetCheck      = "approval_target_entity_type_check"
	coTargetCheck    = "approval_co_target_entity_type_check"
	acquisitionCheck = "contact_acquisition_evidence_source_entity_type_check"
	validateFile     = "core/1795363364_the_target_type_checks_are_validated.up.sql"
	dropFile         = "core/1795363367_an_approval_with_an_old_type_stays_updatable.up.sql"

	// An expired approval needs no decider, and a complete co-target carries an id and a version.
	insertApproval = `INSERT INTO approval (kind, status, proposed_by, proposed_change, diff_hash, expires_at,
			target_entity_type, target_entity_id, co_target_entity_type, co_target_entity_id, co_target_version)
		VALUES ('k', 'expired', 'agent:x', '{}', 'h', now(), $1, uuidv7(), $2::text, CASE WHEN $2::text IS NULL THEN NULL ELSE uuidv7() END,
		        CASE WHEN $2::text IS NULL THEN NULL ELSE 1 END)`
	insertEvidence = `INSERT INTO contact_acquisition_evidence (contact_id, kind, occurred_at, captured_by, source_entity_type, source_entity_id)
		VALUES (uuidv7(), 'requested_quote_or_meeting', now(), 'human:x', $1, uuidv7())`
)

// violated runs the statement in a transaction it rolls back and returns the
// constraint the database named, or "" when the statement went through.
func violated(ctx context.Context, t *testing.T, conn *pgx.Conn, sql string, args ...any) string {
	t.Helper()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
	}()
	_, err = tx.Exec(ctx, sql, args...)
	if err == nil {
		return ""
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("not a database error: %v", err)
	}
	return pgErr.ConstraintName
}

func TestAnApprovalRefusesATargetTypeNoWriterStages(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	ctx := context.Background()
	for _, c := range []struct {
		target   string
		coTarget any
		want     string
	}{
		{"deal", nil, ""},
		{"deal", "stage", ""},
		{"ext_visit_log_visit", nil, ""},
		{"widget", nil, targetCheck},
		{"Deal", nil, targetCheck},
		{"ext:visit-log:visit", nil, targetCheck},
		{"deal", "widget", coTargetCheck},
	} {
		if got := violated(ctx, t, conn, insertApproval, c.target, c.coTarget); got != c.want {
			t.Errorf("target %q, co-target %v: constraint %q, want %q", c.target, c.coTarget, got, c.want)
		}
	}
}

func TestAcquisitionEvidenceCitesAnActivityOrADeal(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	ctx := context.Background()
	for _, c := range []struct{ source, want string }{
		{"note", acquisitionCheck},
		{"Activity", acquisitionCheck},
		// The contact does not exist, so the foreign key refuses these two after the CHECK let them by.
		{"activity", "contact_acquisition_evidence_contact_id_fkey"},
		{"deal", "contact_acquisition_evidence_contact_id_fkey"},
	} {
		if got := violated(ctx, t, conn, insertEvidence, c.source); got != c.want {
			t.Errorf("source %q: constraint %q, want %q", c.source, got, c.want)
		}
	}
}

// reopen puts the three constraints back to not validated, as the file before the
// validating one leaves them, keeping the rows already in the tables.
func reopen(ctx context.Context, t *testing.T, conn *pgx.Conn) {
	t.Helper()
	for _, c := range []struct{ table, name string }{
		{"approval", targetCheck}, {"approval", coTargetCheck}, {"contact_acquisition_evidence", acquisitionCheck},
	} {
		var def string
		if err := conn.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname = $1`, c.name).Scan(&def); err != nil {
			t.Fatalf("reading %s: %v", c.name, err)
		}
		for _, sql := range []string{
			"ALTER TABLE " + c.table + " DROP CONSTRAINT " + c.name,
			"ALTER TABLE " + c.table + " ADD CONSTRAINT " + c.name + " " + def + " NOT VALID",
		} {
			if _, err := conn.Exec(ctx, sql); err != nil {
				t.Fatalf("%s: %v", sql, err)
			}
		}
	}
}

func validated(ctx context.Context, t *testing.T, conn *pgx.Conn, name string) bool {
	t.Helper()
	var ok bool
	if err := conn.QueryRow(ctx, `SELECT convalidated FROM pg_constraint WHERE conname = $1`, name).Scan(&ok); err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return ok
}

func runValidation(ctx context.Context, t *testing.T, conn *pgx.Conn) {
	t.Helper()
	sql, err := os.ReadFile(filepath.Clean(validateFile))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("running the validating migration: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestTheTargetTypeChecksValidateWhenNoRowBreaksThem(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	ctx := context.Background()
	reopen(ctx, t, conn)
	runValidation(ctx, t, conn)
	for _, name := range []string{targetCheck, coTargetCheck, acquisitionCheck} {
		if !validated(ctx, t, conn, name) {
			t.Errorf("%s stayed NOT VALID on a clean table", name)
		}
	}
}

func TestAnOldRowWithAnUnknownTypeKeepsItsCheckNotValidAndTheUpgradeGoesOn(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	ctx := context.Background()
	var def string
	if err := conn.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname = $1`, targetCheck).Scan(&def); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `ALTER TABLE approval DROP CONSTRAINT `+targetCheck); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO approval (kind, status, proposed_by, proposed_change, diff_hash, expires_at, target_entity_type)
		VALUES ('k', 'expired', 'agent:x', '{}', 'h', now(), 'legacy_thing')`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `ALTER TABLE approval ADD CONSTRAINT `+targetCheck+` `+def+` NOT VALID`); err != nil {
		t.Fatal(err)
	}
	runValidation(ctx, t, conn)
	if validated(ctx, t, conn, targetCheck) {
		t.Errorf("%s was validated over a row that breaks it", targetCheck)
	}
	for _, name := range []string{coTargetCheck, acquisitionCheck} {
		if !validated(ctx, t, conn, name) {
			t.Errorf("%s was left NOT VALID though no row breaks it", name)
		}
	}
	if got := violated(ctx, t, conn, insertApproval, "widget", nil); got != targetCheck {
		t.Errorf("a new bad write met constraint %q, want %q", got, targetCheck)
	}
}

func TestAnOldApprovalWithAnUnknownTypeLosesTheCheckAndStaysUpdatable(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	ctx := context.Background()
	var def string
	if err := conn.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname = $1`, targetCheck).Scan(&def); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `ALTER TABLE approval DROP CONSTRAINT `+targetCheck); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO approval (kind, status, proposed_by, proposed_change, diff_hash, expires_at, target_entity_type)
		VALUES ('k', 'expired', 'agent:x', '{}', 'h', now(), 'legacy_thing')`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `ALTER TABLE approval ADD CONSTRAINT `+targetCheck+` `+def+` NOT VALID`); err != nil {
		t.Fatal(err)
	}
	runValidation(ctx, t, conn)
	if _, err := conn.Exec(ctx, `UPDATE approval SET summary = 'x' WHERE target_entity_type = 'legacy_thing'`); err == nil {
		t.Fatal("a NOT VALID constraint should have refused the update: the premise of the next migration is gone")
	}
	raised := runMigration(ctx, t, dropFile)
	if present(ctx, t, conn, targetCheck) {
		t.Errorf("%s was kept over a row that breaks it", targetCheck)
	}
	if len(raised) != 1 || !strings.Contains(raised[0], targetCheck) {
		t.Errorf("want one notice naming %s, got %q", targetCheck, raised)
	}
	for _, name := range []string{coTargetCheck, acquisitionCheck} {
		if !validated(ctx, t, conn, name) {
			t.Errorf("%s was left NOT VALID though no row breaks it", name)
		}
	}
	if _, err := conn.Exec(ctx, `UPDATE approval SET summary = 'x' WHERE target_entity_type = 'legacy_thing'`); err != nil {
		t.Errorf("the legacy approval can no longer change: %v", err)
	}
}

func TestTheApprovalChecksAreValidatedWhenNoRowBreaksThem(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	ctx := context.Background()
	reopen(ctx, t, conn)
	if raised := runMigration(ctx, t, dropFile); len(raised) != 0 {
		t.Errorf("a clean table raised %q", raised)
	}
	for _, name := range []string{targetCheck, coTargetCheck, acquisitionCheck} {
		if !validated(ctx, t, conn, name) {
			t.Errorf("%s stayed NOT VALID on a clean table", name)
		}
	}
}
