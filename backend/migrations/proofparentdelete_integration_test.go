// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestProofHistoryRefusesDirectSubjectDeletion(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	for _, fixture := range []struct {
		parent, table, key, insert string
	}{
		{"contact", "consent_event", "consent_event_contact_id_fkey", `INSERT INTO consent_event
		 (contact_id, purpose_id, new_state, captured_at, source, captured_by)
		 VALUES (@parent, @purpose, 'granted', now(), 'manual', 'test')`},
		{"lead", "consent_event", "consent_event_lead_id_fkey", `INSERT INTO consent_event
		 (lead_id, purpose_id, new_state, captured_at, source, captured_by)
		 VALUES (@parent, @purpose, 'granted', now(), 'manual', 'test')`},
		{"contact", "consent_qualifying_event", "consent_qualifying_event_contact_fkey", `INSERT INTO consent_qualifying_event
		 (contact_id, kind, occurred_at, source, captured_by, note)
		 VALUES (@parent, 'in_person', now(), 'manual', 'test', 'Agreed during the meeting')`},
		{"lead", "consent_qualifying_event", "consent_qualifying_event_lead_id_fkey", `INSERT INTO consent_qualifying_event
		 (lead_id, kind, occurred_at, source, captured_by, note)
		 VALUES (@parent, 'in_person', now(), 'manual', 'test', 'Agreed during the meeting')`},
		{"contact", "contact_acquisition_evidence", "contact_acquisition_evidence_contact_id_fkey", `INSERT INTO contact_acquisition_evidence
		 (contact_id, kind, occurred_at, captured_by) VALUES (@parent, 'in_person_permission', now(), 'test')`},
		{"contact", "provider_run", "provider_run_contact_id_fkey", `INSERT INTO provider_run
		 (subject_kind, contact_id, provider, trigger, state, input_fingerprint, external_correlation_id,
		 connection_version, connection_epoch, configuration_snapshot, requested_categories, completed_at)
		 VALUES ('contact', @parent, 'fixture', 'manual', 'completed', 'fixture', uuidv7(), 1, 1, '{}', '{}', now())`},
	} {
		t.Run(fixture.key, func(t *testing.T) {
			ctx := t.Context()
			tx, err := conn.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer rollbackProofFixture(ctx, t, tx)
			var parent, purpose string
			if err := tx.QueryRow(ctx, `INSERT INTO `+pgx.Identifier{fixture.parent}.Sanitize()+`
			 (full_name, source, captured_by) VALUES ('Proof Subject', 'manual', 'test') RETURNING id`).Scan(&parent); err != nil {
				t.Fatal(err)
			}
			if err := tx.QueryRow(ctx, `INSERT INTO consent_purpose (key, label)
			 VALUES ($1, 'Proof fixture') RETURNING id`, parent).Scan(&purpose); err != nil {
				t.Fatal(err)
			}
			var child string
			if err := tx.QueryRow(ctx, fixture.insert+" RETURNING id", pgx.NamedArgs{"parent": parent, "purpose": purpose}).Scan(&child); err != nil {
				t.Fatal(err)
			}
			attempt, err := tx.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			_, deletion := attempt.Exec(ctx, `DELETE FROM `+pgx.Identifier{fixture.parent}.Sanitize()+` WHERE id = $1`, parent)
			rollbackProofFixture(ctx, t, attempt)
			constraint, refused := errors.AsType[*pgconn.PgError](deletion)
			if !refused || constraint.Code != "23503" || constraint.ConstraintName != fixture.key {
				t.Fatalf("direct deletion returned %v, want a refusal from %s", deletion, fixture.key)
			}
			var kept int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM `+pgx.Identifier{fixture.table}.Sanitize()+` WHERE id = $1`, child).Scan(&kept); err != nil {
				t.Fatal(err)
			}
			if kept != 1 {
				t.Fatal("the failed deletion lost its history")
			}
		})
	}
}

func rollbackProofFixture(ctx context.Context, t *testing.T, tx pgx.Tx) {
	t.Helper()
	if err := tx.Rollback(ctx); err != nil {
		t.Errorf("rolling back the proof fixture: %v", err)
	}
}

func TestEveryProofSubjectKeyIsValidatedAndRestrictsDeletion(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	rows, err := conn.Query(t.Context(), `SELECT conrelid::regclass::text || '.' || conname,
	 confdeltype::text = 'r' AND convalidated FROM pg_constraint
	 WHERE contype = 'f' AND conrelid IN ('consent_event'::regclass, 'consent_qualifying_event'::regclass,
	 'contact_acquisition_evidence'::regclass, 'provider_run'::regclass)
	 AND confrelid IN ('contact'::regclass, 'lead'::regclass)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var key string
		var protected bool
		if err := rows.Scan(&key, &protected); err != nil {
			t.Fatal(err)
		}
		count++
		if !protected {
			t.Errorf("%s must be validated and refuse direct subject deletion", key)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count < 6 {
		t.Fatalf("found only %d subject keys, want at least the six proof and run references", count)
	}
}
