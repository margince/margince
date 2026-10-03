// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

const capturedMailAnswersUp = "datafix/2026-10-03_open_duties_the_captured_mail_already_answers.up.sql"

// owedContact seeds a contact at `email` whose acquisition `capturedBy` wrote
// as unknown_legacy, with an open Art. 14 case, and answers the case id.
func owedContact(ctx context.Context, t *testing.T, conn *pgx.Conn, email, capturedBy string) string {
	t.Helper()
	var caseID string
	if err := conn.QueryRow(ctx, `
WITH c AS (
  INSERT INTO contact (full_name, source, captured_by) VALUES ($1, 'gmail', 'connector:gmail') RETURNING id
), ce AS (
  INSERT INTO contact_email (contact_id, email, source, captured_by)
  SELECT id, $1, 'gmail', 'connector:gmail' FROM c RETURNING contact_id
), e AS (
  INSERT INTO contact_acquisition_evidence (contact_id, kind, occurred_at, captured_by)
  SELECT id, 'unknown_legacy', now(), $2 FROM c RETURNING id, contact_id
)
INSERT INTO privacy_notice_case (contact_id, acquisition_id, rule, due_at, state)
SELECT contact_id, id, 'art14', now() + interval '1 month', 'open' FROM e RETURNING id`,
		email, capturedBy).Scan(&caseID); err != nil {
		t.Fatal(err)
	}
	return caseID
}

// capturedMail seeds one mail the seat's mailbox captured, with its participants
// given as role → address ("from" may be the seat itself, as "seat").
func capturedMail(ctx context.Context, t *testing.T, conn *pgx.Conn, seat, direction string, attested bool,
	dated time.Time, parties map[string]string) {
	t.Helper()
	var id string
	if err := conn.QueryRow(ctx, `
		INSERT INTO activity (kind, subject, direction, occurred_at, source_system, source_id, source, captured_by,
		                      counterparty_outbound_attested)
		VALUES ('email', 'hi', $2, $3, 'gmail', gen_random_uuid()::text, 'gmail:seed', 'connector:gmail:' || $1, $4)
		RETURNING id`, seat, direction, dated, attested).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, id, seat); err != nil {
		t.Fatal(err)
	}
	for role, address := range parties {
		var err error
		if address == "seat" {
			_, err = conn.Exec(ctx, `INSERT INTO activity_participant (activity_id, role, user_id) VALUES ($1, $2, $3)`,
				id, role, seat)
		} else {
			_, err = conn.Exec(ctx, `INSERT INTO activity_participant (activity_id, role, address) VALUES ($1, $2, $3)`,
				id, role, address)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestOpenDutiesTheCapturedMailAlreadyAnswersAreSettled(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()
	var seat string
	if err := conn.QueryRow(ctx,
		`INSERT INTO app_user (email, display_name) VALUES ('rep@answers.test', 'Rep') RETURNING id`).Scan(&seat); err != nil {
		t.Fatal(err)
	}
	connected := time.Now().AddDate(0, -1, 0)
	if _, err := conn.Exec(ctx, `
		INSERT INTO capture_connection (provider, user_id, status, created_at) VALUES ('gmail', $1, 'connected', $2)`,
		seat, connected); err != nil {
		t.Fatal(err)
	}
	old := connected.AddDate(-1, 0, 0)
	const verdict = "agent:capture_counterparty_verdict"

	wrote := owedContact(ctx, t, conn, "wrote@customer.test", verdict)
	capturedMail(ctx, t, conn, seat, "inbound", false, old, map[string]string{"from": "wrote@customer.test", "to": "seat"})

	copied := owedContact(ctx, t, conn, "copied@customer.test", verdict)
	capturedMail(ctx, t, conn, seat, "outbound", true, old,
		map[string]string{"from": "seat", "to": "addressed@customer.test", "cc": "copied@customer.test"})

	// Copied on mail sent after the connection: a new acquisition.
	late := owedContact(ctx, t, conn, "late@customer.test", verdict)
	capturedMail(ctx, t, conn, seat, "outbound", true, time.Now(),
		map[string]string{"from": "seat", "to": "addressed2@customer.test", "cc": "late@customer.test"})

	// An unknown a seat stated is a claim about somewhere else.
	stated := owedContact(ctx, t, conn, "stated@customer.test", "human:"+seat)
	capturedMail(ctx, t, conn, seat, "inbound", false, old, map[string]string{"from": "stated@customer.test", "to": "seat"})

	execFile(ctx, t, conn, capturedMailAnswersUp)
	execFile(ctx, t, conn, capturedMailAnswersUp)

	for name, c := range map[string]struct{ id, state string }{
		"a verdict-made contact who wrote to us":      {wrote, "exempt_with_reason"},
		"a contact copied on mail sent before":        {copied, "exempt_with_reason"},
		"a contact copied on mail sent after":         {late, "open"},
		"an unknown a seat stated, though they wrote": {stated, "open"},
	} {
		var state string
		if err := conn.QueryRow(ctx, `SELECT state FROM privacy_notice_case WHERE id = $1`, c.id).Scan(&state); err != nil {
			t.Fatal(err)
		}
		if state != c.state {
			t.Errorf("%s: case is %s, want %s", name, state, c.state)
		}
	}
	var subjectRows int
	if err := conn.QueryRow(ctx, `
		SELECT count(*) FROM contact_acquisition_evidence e JOIN contact_email ce ON ce.contact_id = e.contact_id
		 WHERE ce.email = 'wrote@customer.test' AND e.kind = 'subject_initiated'`).Scan(&subjectRows); err != nil {
		t.Fatal(err)
	}
	if subjectRows != 1 {
		t.Errorf("the contact who wrote has %d subject_initiated acquisitions, want exactly 1", subjectRows)
	}
}
