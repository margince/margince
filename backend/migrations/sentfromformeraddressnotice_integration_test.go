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

const sentFromFormerUp = "datafix/2026-10-03_mail_sent_from_a_former_address_owes_no_notice.up.sql"

// formerSend describes one message captured inbound from the seat's own other
// address, naming the contact on its To line.
type formerSend struct {
	from     string // the From the message was captured under
	original string // the stored RFC822 original
	dated    time.Time
}

// sentFromFormerCase seeds one capture-made contact on the To line of such a
// message, with an open Art. 14 case on an unknown_legacy acquisition, and
// answers the case id.
func sentFromFormerCase(ctx context.Context, t *testing.T, conn *pgx.Conn, seat, email string, m formerSend) string {
	t.Helper()
	var caseID string
	err := conn.QueryRow(ctx, `
WITH c AS (
  INSERT INTO contact (full_name, source, captured_by) VALUES ($2, 'gmail', 'connector:gmail') RETURNING id
), ce AS (
  INSERT INTO contact_email (contact_id, email, source, captured_by)
  SELECT id, $2, 'gmail', 'connector:gmail' FROM c RETURNING contact_id
), rc AS (
  INSERT INTO raw_capture (source_system, source_id, payload) VALUES ('email', $2, to_jsonb($4::text)) RETURNING id
), a AS (
  INSERT INTO activity (kind, subject, direction, occurred_at, source_system, source_id, source, captured_by,
                        counterparty_email, raw_capture_id)
  VALUES ('email', 'hi', 'inbound', $5, 'gmail', $2, 'gmail:seed', 'connector:gmail:' || $1,
          $3, (SELECT id FROM rc)) RETURNING id
), p AS (
  INSERT INTO activity_participant (activity_id, role, address, user_id)
  SELECT id, 'to', $2, NULL::uuid FROM a
  UNION ALL SELECT id, 'to', NULL, $1::uuid FROM a
  RETURNING activity_id
), ci AS (
  INSERT INTO capture_import (activity_id, user_id) SELECT id, $1::uuid FROM a RETURNING id
), e AS (
  INSERT INTO contact_acquisition_evidence (contact_id, kind, occurred_at, captured_by)
  SELECT id, 'unknown_legacy', $5, 'agent:capture_counterparty_verdict' FROM c RETURNING id, contact_id
)
INSERT INTO privacy_notice_case (contact_id, acquisition_id, rule, due_at, state)
SELECT e.contact_id, e.id, 'art14', $5::timestamptz + interval '1 month', 'open' FROM e RETURNING id`,
		seat, email, m.from, m.original, m.dated).Scan(&caseID)
	if err != nil {
		t.Fatal(err)
	}
	return caseID
}

func sentOriginal(from, to string, received bool) string {
	head := ""
	if received {
		head = "Received: by 2002:a05:7300:1 with SMTP id x1; Mon, 2 Jan 2023 10:00:00 +0000\r\n"
	}
	return head + "From: " + from + "\r\nTo: " + to + "\r\nDate: Mon, 2 Jan 2023 10:00:00 +0000\r\n\r\nhello\r\n"
}

func TestMailSentFromAFormerAddressClosesTheDutiesOfItsRecipients(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()
	var seat string
	if err := conn.QueryRow(ctx,
		`INSERT INTO app_user (email, display_name) VALUES ('rep@former.test', 'Rep') RETURNING id`).Scan(&seat); err != nil {
		t.Fatal(err)
	}
	connected := time.Now().AddDate(0, -1, 0)
	if _, err := conn.Exec(ctx, `
		INSERT INTO capture_connection (provider, user_id, status, created_at) VALUES ('gmail', $1, 'connected', $2)`,
		seat, connected); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO capture_owner_identity (user_id, kind, value, created_by)
		VALUES ($1, 'address', 'rep@old-employer.test', 'test')`, seat); err != nil {
		t.Fatal(err)
	}
	old := connected.AddDate(-2, 0, 0)
	const own, stranger = "rep@old-employer.test", "someone@old-employer.test"

	cases := map[string]struct{ id, wantState, wantKind string }{
		// Written into the mailbox by its owner: no hop ever delivered it.
		"sent from the former address": {
			sentFromFormerCase(ctx, t, conn, seat, "to@customer.test",
				formerSend{from: own, original: sentOriginal(own, "to@customer.test", false), dated: old}),
			"exempt_with_reason", "mailbox_history",
		},
		// Delivered to the mailbox: mail from the former address that arrived,
		// which the received-mail rule judges, not this one.
		"delivered from the former address": {
			sentFromFormerCase(ctx, t, conn, seat, "delivered@customer.test",
				formerSend{from: own, original: sentOriginal(own, "delivered@customer.test", true), dated: old}),
			"open", "unknown_legacy",
		},
		// An address the seat never claimed is somebody else's mail.
		"sent from an unclaimed address": {
			sentFromFormerCase(ctx, t, conn, seat, "theirs@customer.test",
				formerSend{from: stranger, original: sentOriginal(stranger, "theirs@customer.test", false), dated: old}),
			"open", "unknown_legacy",
		},
		// Written after the connection: a new acquisition, and the duty stays.
		"sent after connecting": {
			sentFromFormerCase(ctx, t, conn, seat, "new@customer.test",
				formerSend{from: own, original: sentOriginal(own, "new@customer.test", false), dated: time.Now()}),
			"open", "unknown_legacy",
		},
	}

	execFile(ctx, t, conn, sentFromFormerUp)
	// Running it again changes nothing.
	execFile(ctx, t, conn, sentFromFormerUp)

	for name, c := range cases {
		if state, kind := caseAndKind(ctx, t, conn, c.id); state != c.wantState || kind != c.wantKind {
			t.Errorf("%s: case is %s on a %s acquisition, want %s on %s", name, state, kind, c.wantState, c.wantKind)
		}
	}
}
