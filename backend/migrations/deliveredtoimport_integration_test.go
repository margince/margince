// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

const deliveredToImportUp = "datafix/2026-10-03-2_mail_delivered_to_a_seat_is_their_import.up.sql"

// capturedListMail seeds one message the seat's Gmail connection captured with
// no import row, its stored original carrying `head` above a list's headers,
// and answers the activity id.
func capturedListMail(ctx context.Context, t *testing.T, conn *pgx.Conn, seat, msgID, head string) string {
	t.Helper()
	return capturedListMailHeld(ctx, t, conn, seat, msgID, head, "workspace", "")
}

// capturedListMailHeld is capturedListMail stored at a given audience, as a
// held or classified mailbox stores it, with the hold's reason.
func capturedListMailHeld(
	ctx context.Context, t *testing.T, conn *pgx.Conn, seat, msgID, head, audience, reason string,
) string {
	t.Helper()
	var id string
	err := conn.QueryRow(ctx, `
WITH rc AS (
  INSERT INTO raw_capture (source_system, source_id, payload)
  VALUES ('email', $2, to_jsonb($3::text)) RETURNING id
)
INSERT INTO activity (kind, subject, direction, occurred_at, source_system, source_id, source, captured_by,
                      counterparty_email, raw_capture_id, audience, audience_reason)
VALUES ('email', 'list', 'inbound', now(), 'gmail', $2, 'gmail:seed', 'connector:gmail:' || $1,
        'organiser@customer.test', (SELECT id FROM rc), $4, NULLIF($5, '')) RETURNING id`,
		seat, msgID, head+"From: organiser@customer.test\r\nTo: all@customer.test\r\n\r\nhello\r\n",
		audience, reason).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func importRows(ctx context.Context, t *testing.T, conn *pgx.Conn, activity, seat string) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(ctx,
		`SELECT count(*) FROM capture_import WHERE activity_id = $1 AND user_id = $2`, activity, seat).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestMailDeliveredToTheSeatGetsTheImportRowCaptureNeverWrote(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()
	var seat string
	if err := conn.QueryRow(ctx,
		`INSERT INTO app_user (email, display_name) VALUES ('rep@list.test', 'Rep') RETURNING id`).Scan(&seat); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO capture_owner_identity (user_id, kind, value, created_by)
		VALUES ($1, 'address', 'rep@alias.test', 'test')`, seat); err != nil {
		t.Fatal(err)
	}
	const hop = "Received: by 2002:a05:7300:1 with SMTP id x1; Mon, 2 Jan 2023 10:00:00 +0000\r\n"

	cases := map[string]struct {
		id   string
		want int
	}{
		"delivered to the seat's alias": {
			capturedListMail(ctx, t, conn, seat, "a1@list.test", "Delivered-To: rep@alias.test\r\n"+hop), 1,
		},
		// Below the receiving hop: the sender's own text.
		"a Delivered-To the sender wrote": {
			capturedListMail(ctx, t, conn, seat, "a2@list.test", hop+"Delivered-To: rep@alias.test\r\n"), 0,
		},
		"delivered to an address the seat does not hold": {
			capturedListMail(ctx, t, conn, seat, "a3@list.test", "Delivered-To: other@alias.test\r\n"+hop), 0,
		},
	}

	execFile(ctx, t, conn, deliveredToImportUp)
	// Running it again changes nothing.
	execFile(ctx, t, conn, deliveredToImportUp)

	for name, c := range cases {
		if n := importRows(ctx, t, conn, c.id, seat); n != c.want {
			t.Errorf("%s: %d import rows for the seat, want %d", name, n, c.want)
		}
	}
}

// An import row is a vote in the message's audience. The backfill must cast the
// vote that keeps a held message held: the mailbox's posture and the hold's own
// reason, read by activities.contributionOf as participants. Where the mailbox
// is shared, the only vote it could cast opens the message, so it casts none.
func TestTheImportBackfillKeepsAHeldMessageHeld(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()
	const hop = "Received: by 2002:a05:7300:1 with SMTP id x1; Mon, 2 Jan 2023 10:00:00 +0000\r\n"
	seat := func(email, posture string) string {
		var id string
		if err := conn.QueryRow(ctx,
			`INSERT INTO app_user (email, display_name) VALUES ($1, 'Rep') RETURNING id`, email).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(ctx, `
			INSERT INTO capture_connection (provider, user_id, status, account_label, mail_posture)
			VALUES ('gmail', $1, 'connected', $2, $3)`, id, email, posture); err != nil {
			t.Fatal(err)
		}
		return id
	}
	classifiedSeat := seat("classified@list.test", "classified")
	sharedSeat := seat("shared@list.test", "shared")

	heldID := capturedListMailHeld(ctx, t, conn, classifiedSeat, "h1@list.test",
		"Delivered-To: classified@list.test\r\n"+hop, "participants", "posture")
	sharedID := capturedListMailHeld(ctx, t, conn, sharedSeat, "h2@list.test",
		"Delivered-To: shared@list.test\r\n"+hop, "participants", "posture")

	execFile(ctx, t, conn, deliveredToImportUp)

	var posture, reason string
	if err := conn.QueryRow(ctx, `
		SELECT coalesce(posture_at_import, ''), coalesce(verdict_reason, '') FROM capture_import
		 WHERE activity_id = $1 AND user_id = $2`, heldID, classifiedSeat).Scan(&posture, &reason); err != nil {
		t.Fatalf("the classified mailbox's held message got no import row: %v", err)
	}
	if posture != "classified" || reason != "posture" {
		t.Errorf("the held message's import row votes posture %q, reason %q; want classified, posture", posture, reason)
	}
	if n := importRows(ctx, t, conn, sharedID, sharedSeat); n != 0 {
		t.Errorf("a held message in a shared mailbox got %d import rows, which would vote it open; want 0", n)
	}
}
