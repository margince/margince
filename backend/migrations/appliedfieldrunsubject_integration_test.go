// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// A filled field belongs to the run that filled it.
//
// provider_applied_field carries contact_id, provider and run_id as
// independent columns, and nothing required the first two to agree with the
// provider_run the third names. A revert reads these rows to decide what to
// clear, so a row whose contact does not match its run clears a field on the
// wrong contact — and the unique index keys a scalar marker on the CONTACT, so
// one run naming two contacts carries two markers for one field.
//
// Both directions, because a constraint that refused everything would pass the
// refusal half on its own.

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestAFilledFieldMustBelongToTheRunItNames(t *testing.T) {
	ownerDSN, _ := dsns(t)
	owner := connect(t, ownerDSN)
	headSchema(t, owner)
	ctx := context.Background()

	subject := seedAppliedFieldContact(t, owner, "Ada Lindqvist")
	other := seedAppliedFieldContact(t, owner, "Ida Keller")
	run := seedProviderRun(t, owner, subject, "surfe")

	// THE REFUSAL: the run is about `subject`, the marker claims `other`.
	if err := insertAppliedField(ctx, owner, run, other, "surfe"); err == nil {
		t.Fatal("a marker naming a contact its run is not about was accepted — a revert " +
			"reading it clears a bought field on somebody the purchase never touched")
	} else if !strings.Contains(err.Error(), "provider_applied_field_run_subject_fkey") {
		t.Fatalf("refused by something other than the run-subject binding: %v", err)
	}

	// The PROVIDER half, same row, one column moved: the run is surfe's.
	if err := insertAppliedField(ctx, owner, run, subject, "otherco"); err == nil {
		t.Error("a marker naming a provider its run is not from was accepted")
	}

	// THE ALLOW ARM: contact and provider both agreeing with the run.
	if err := insertAppliedField(ctx, owner, run, subject, "surfe"); err != nil {
		t.Fatalf("a marker agreeing with its run was refused: %v", err)
	}
}

// And the ordering erasure already keeps is now the ordering it MUST keep: a
// scrubbed run names no contact, so a marker still pointing at it has nothing
// to match. Both erasure paths delete the markers before scrubbing the run;
// this is what stops that becoming optional.
func TestScrubbingARunAfterItsMarkersAreGoneIsStillAllowed(t *testing.T) {
	ownerDSN, _ := dsns(t)
	owner := connect(t, ownerDSN)
	headSchema(t, owner)
	ctx := context.Background()

	subject := seedAppliedFieldContact(t, owner, "Lars Brandt")
	run := seedProviderRun(t, owner, subject, "surfe")
	if err := insertAppliedField(ctx, owner, run, subject, "surfe"); err != nil {
		t.Fatalf("seeding the marker: %v", err)
	}

	// The scrub WITHOUT the delete is refused, which is the ordering stated.
	if _, err := owner.Exec(ctx, `
		UPDATE provider_run SET contact_id = NULL, subject_kind = 'scrubbed' WHERE id = $1`,
		run); err == nil {
		t.Fatal("a run was scrubbed out from under a live marker — the marker now names a " +
			"contact the run does not, which is the state this binding exists to refuse")
	}

	// And with it, in the order both erasure paths already use.
	if _, err := owner.Exec(ctx,
		`DELETE FROM provider_applied_field WHERE run_id = $1`, run); err != nil {
		t.Fatalf("deleting the marker: %v", err)
	}
	if _, err := owner.Exec(ctx, `
		UPDATE provider_run SET contact_id = NULL, subject_kind = 'scrubbed' WHERE id = $1`,
		run); err != nil {
		t.Fatalf("scrubbing a run whose markers are gone was refused: %v", err)
	}
}

func insertAppliedField(ctx context.Context, conn *pgx.Conn, run, contact, provider string) error {
	_, err := conn.Exec(ctx, `
		INSERT INTO provider_applied_field
		       (run_id, contact_id, provider, target_table, target_field, applied_value, captured_by)
		VALUES ($1, $2, $3, 'contact', 'title', 'Head of Ops', 'human:seed')`,
		run, contact, provider)
	return err
}

func seedAppliedFieldContact(t *testing.T, conn *pgx.Conn, name string) string {
	t.Helper()
	var id string
	if err := conn.QueryRow(context.Background(), `
		INSERT INTO contact (full_name, source, captured_by)
		VALUES ($1, 'seed', 'human:seed') RETURNING id::text`, name).Scan(&id); err != nil {
		t.Fatalf("seeding the contact %q: %v", name, err)
	}
	return id
}

func seedProviderRun(t *testing.T, conn *pgx.Conn, contact, provider string) string {
	t.Helper()
	var id string
	if err := conn.QueryRow(context.Background(), `
		INSERT INTO provider_run
		  (subject_kind, contact_id, provider, trigger, state, input_fingerprint,
		   external_correlation_id, connection_version, connection_epoch,
		   configuration_snapshot, requested_categories, completed_at)
		VALUES ('contact', $1, $2, 'manual', 'completed', $3,
		        gen_random_uuid(), 1, 1, '{}'::jsonb, ARRAY['professional_email'], now())
		RETURNING id::text`, contact, provider, contact).Scan(&id); err != nil {
		t.Fatalf("seeding the provider run: %v", err)
	}
	return id
}
