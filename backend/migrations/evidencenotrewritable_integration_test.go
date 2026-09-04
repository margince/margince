// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// A proof row the application can edit is not a proof.
//
// consent_event says what a person was shown and what they answered.
// communication_decision says why a message was allowed to go out. Both are
// served to a data subject as evidence, so the runtime role holding a general
// UPDATE on them was a standing invitation for any defect or repair script to
// rewrite the history that settles the question.
//
// Two halves, and BOTH are needed. Proving the rewrite is refused says nothing
// about whether Art. 17 still reaches the address a message went to — and a
// permission change that quietly broke erasure would be a worse defect than the
// one it fixed.

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// evidenceRow seeds one decision and one consent event through the owner, and
// answers the ids the app role then tries to change.
type evidenceRow struct {
	person   string
	decision string
	event    string
}

func seedEvidence(t *testing.T, owner *pgx.Conn) evidenceRow {
	t.Helper()
	ctx := context.Background()
	var e evidenceRow
	if err := owner.QueryRow(ctx,
		`INSERT INTO person (full_name, source, captured_by)
		 VALUES ('Evidence Subject', 'manual', 'human:seed') RETURNING id`).Scan(&e.person); err != nil {
		t.Fatalf("seeding the person: %v", err)
	}
	var purpose string
	if err := owner.QueryRow(ctx,
		`INSERT INTO consent_purpose (key, label, class, requires_double_opt_in)
		 VALUES ('evidence_news', 'Newsletter', 'marketing', false) RETURNING id`).Scan(&purpose); err != nil {
		t.Fatalf("seeding the purpose: %v", err)
	}
	if err := owner.QueryRow(ctx, `
		INSERT INTO consent_event
		    (person_id, purpose_id, new_state, source, captured_by, captured_at, policy_text, policy_version)
		VALUES ($1, $2, 'granted', 'preference_center', 'human:subject', now(),
		        'You agreed to receive our newsletter.', 'v1')
		RETURNING id`, e.person, purpose).Scan(&e.event); err != nil {
		t.Fatalf("seeding the consent proof: %v", err)
	}

	var user, activity, delivery string
	if err := owner.QueryRow(ctx,
		`INSERT INTO app_user (email, display_name) VALUES ('rep@evidence.test', 'Rep') RETURNING id`).
		Scan(&user); err != nil {
		t.Fatalf("seeding the sender: %v", err)
	}
	if err := owner.QueryRow(ctx,
		`INSERT INTO activity (kind, direction, subject, occurred_at, source, captured_by)
		 VALUES ('email', 'outbound', 'Hello', now(), 'manual', 'human:seed') RETURNING id`).
		Scan(&activity); err != nil {
		t.Fatalf("seeding the activity: %v", err)
	}
	if err := owner.QueryRow(ctx, `
		INSERT INTO comms_outbound
		    (id, activity_id, user_id, provider, message_id,
		     recipients, cc, references_chain, subject, body, consent_purpose)
		VALUES (gen_random_uuid(), $1, $2, 'gmail', 'mid-evidence',
		        '["subject@evidence.test"]'::jsonb, '[]'::jsonb, '[]'::jsonb,
		        'Hello', 'body', 'evidence_news')
		RETURNING id`, activity, user).Scan(&delivery); err != nil {
		t.Fatalf("seeding the delivery: %v", err)
	}
	if err := owner.QueryRow(ctx, `
		INSERT INTO communication_decision
		    (delivery_id, attempt, decision_set_id, recipient_address, subject_kind, subject_id,
		     phase, resolved_category, verdict, reason_code, mode, actor)
		VALUES ($1, 0, gen_random_uuid(), 'subject@evidence.test', 'person', $2,
		        'staging', 'marketing', 'allow', 'allowed', 'enforce', 'human:seed')
		RETURNING id`, delivery, e.person).Scan(&e.decision); err != nil {
		t.Fatalf("seeding the decision: %v", err)
	}
	return e
}

// TestTheRuntimeRoleCannotRewriteAFinding is the invariant this migration turns
// from a comment into a permission.
//
// Every column below is a FINDING — what the engine concluded, and what the
// person was told. None of them has a legitimate writer in this tree.
//
// Mutation: drop either REVOKE from the up migration and the matching case
// stops being refused.
func TestTheRuntimeRoleCannotRewriteAFinding(t *testing.T) {
	ownerDSN, appDSN := dsns(t)
	owner := connect(t, ownerDSN)
	headSchema(t, owner)
	e := seedEvidence(t, owner)
	app := connect(t, appDSN)
	ctx := context.Background()

	for _, c := range []struct {
		what string
		sql  string
	}{
		{"the verdict on a send", `UPDATE communication_decision SET verdict = 'deny' WHERE id = $1`},
		{"the reason a send was allowed", `UPDATE communication_decision SET reason_code = 'invented' WHERE id = $1`},
		{"the category a message was resolved as", `UPDATE communication_decision SET resolved_category = 'marketing' WHERE id = $1`},
		{"a decision row outright", `DELETE FROM communication_decision WHERE id = $1`},
	} {
		if _, err := app.Exec(ctx, c.sql, e.decision); !permissionDenied(err) {
			t.Errorf("the runtime role could change %s: %v — an evidence row it can edit is not evidence", c.what, err)
		}
	}

	for _, c := range []struct {
		what string
		sql  string
	}{
		{"what a person answered", `UPDATE consent_event SET new_state = 'withdrawn' WHERE id = $1`},
		{"the wording a person was shown", `UPDATE consent_event SET policy_text = 'something else' WHERE id = $1`},
		{"who recorded a consent", `UPDATE consent_event SET captured_by = 'human:someone-else' WHERE id = $1`},
		{"a proof row outright", `DELETE FROM consent_event WHERE id = $1`},
	} {
		if _, err := app.Exec(ctx, c.sql, e.event); !permissionDenied(err) {
			t.Errorf("the runtime role could change %s: %v", c.what, err)
		}
	}
}

// TestErasureAndTheLeadCarryStillReachTheirColumns is the other half, and the
// reason the REVOKE is column-scoped rather than blanket.
//
// Three writers touch communication_decision and one touches consent_event, and
// every one of them changes only which subject a row points at:
// privacy/erasure_consent.go, erasure_leadtwins.go and retentionactions.go
// tombstone the address and null the subject link; people/consentcarry.go
// re-points a proof onto the surviving record when a lead is promoted.
//
// A blanket REVOKE would have passed the test above and broken Art. 17, which
// is the worse defect.
//
// Mutation: drop either GRANT from the up migration and the matching statement
// starts being refused.
func TestErasureAndTheLeadCarryStillReachTheirColumns(t *testing.T) {
	ownerDSN, appDSN := dsns(t)
	owner := connect(t, ownerDSN)
	headSchema(t, owner)
	e := seedEvidence(t, owner)
	app := connect(t, appDSN)
	ctx := context.Background()

	// Erasure's own statement, as privacy/erasure_consent.go writes it.
	if _, err := app.Exec(ctx, `
		UPDATE communication_decision
		   SET recipient_address = 'erased+' || id || '@example.invalid',
		       subject_id = NULL, subject_kind = NULL
		 WHERE subject_id = $1`, e.person); err != nil {
		t.Errorf("erasure can no longer retire a subject's decisions: %v — Art. 17 has to reach the address a message went to", err)
	}

	// The lead carry's own statement, as people/consentcarry.go writes it.
	if _, err := app.Exec(ctx,
		`UPDATE consent_event SET person_id = $2 WHERE person_id = $1`, e.person, e.person); err != nil {
		t.Errorf("the lead-promotion carry can no longer re-point a proof row: %v", err)
	}
}

// permissionDenied reads the SQLSTATE rather than the message, so a Postgres
// upgrade that rewords the error does not turn this test green.
func permissionDenied(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "42501") ||
		strings.Contains(strings.ToLower(err.Error()), "permission denied")
}
