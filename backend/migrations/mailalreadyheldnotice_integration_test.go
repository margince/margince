// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	arrivalUp    = "core/1790871110_an_import_row_records_when_the_provider_says_mail_arrived.up.sql"
	arrivalDown  = "core/1790871110_an_import_row_records_when_the_provider_says_mail_arrived.down.sql"
	heldMailUp   = "datafix/1790871111_mail_a_mailbox_already_held_owes_no_notice.up.sql"
	heldMailDown = "datafix/1790871111_mail_a_mailbox_already_held_owes_no_notice.down.sql"
)

// receivedMail describes one inbound message that named the contact on Cc.
type receivedMail struct {
	headerDate time.Time  // the Date header, which the sender writes
	providerAt *time.Time // the provider's arrival time, nil when none is on record
	bulk       bool
	original   string // the stored RFC822 original, empty for none
	enveloped  bool   // stored base64 in capture's envelope, as non-UTF-8 mail is
	payload    string // a stored original given verbatim as jsonb, overriding original
	landedBy   string // the activity's captured_by; empty is this seat's Gmail connection
	state      string // the case's state; empty is open
}

// receivedCase seeds one captured contact that only ever appeared on the Cc
// line of a message the seat's mailbox received, with an open Art. 14 case on
// an unknown_legacy acquisition stamped by `capturedBy`. It answers the case id.
func receivedCase(
	ctx context.Context, t *testing.T, conn *pgx.Conn,
	seat, email, capturedBy string, m receivedMail,
) string {
	t.Helper()
	var caseID string
	err := conn.QueryRow(ctx, `
WITH c AS (
  INSERT INTO contact (full_name, source, captured_by) VALUES ($2, 'gmail', 'connector:gmail') RETURNING id
), ce AS (
  INSERT INTO contact_email (contact_id, email, source, captured_by)
  SELECT id, $2, 'gmail', 'connector:gmail' FROM c RETURNING contact_id
), rc AS (
  INSERT INTO raw_capture (source_system, source_id, payload)
  SELECT 'email', $2,
         CASE WHEN $9 <> '' THEN $9::jsonb
              WHEN $8 THEN jsonb_build_object('encoding', 'base64',
                                              'data', translate(encode(convert_to($7::text, 'UTF8'), 'base64'), E'\n', ''))
              ELSE to_jsonb($7::text) END
   WHERE $7 <> '' OR $9 <> '' RETURNING id
), a AS (
  INSERT INTO activity (kind, subject, direction, occurred_at, source_system, source_id, source, captured_by,
                        counterparty_email, bulk_mail_attested, raw_capture_id)
  VALUES ('email', 'hi', 'inbound', $4, 'gmail', $2, 'gmail:seed', coalesce(NULLIF($10, ''), 'connector:gmail:' || $1),
          'sender@elsewhere.test', $6, (SELECT id FROM rc)) RETURNING id
), p AS (
  INSERT INTO activity_participant (activity_id, role, address, user_id)
  SELECT id, 'from', 'sender@elsewhere.test', NULL::uuid FROM a
  UNION ALL SELECT id, 'cc', $2, NULL::uuid FROM a
  UNION ALL SELECT id, 'to', NULL, $1::uuid FROM a
  RETURNING activity_id
), ci AS (
  INSERT INTO capture_import (activity_id, user_id, provider_received_at) SELECT id, $1::uuid, $5 FROM a RETURNING id
), e AS (
  INSERT INTO contact_acquisition_evidence (contact_id, kind, occurred_at, captured_by)
  SELECT id, 'unknown_legacy', $4, $3 FROM c RETURNING id, contact_id
)
INSERT INTO privacy_notice_case (contact_id, acquisition_id, rule, due_at, state, blocked_reason)
SELECT e.contact_id, e.id, 'art14', $4::timestamptz + interval '1 month', coalesce(NULLIF($11, ''), 'open'),
       CASE WHEN $11 = 'blocked' THEN 'no address on file' END
  FROM e RETURNING id`,
		seat, email, capturedBy, m.headerDate, m.providerAt, m.bulk, m.original, m.enveloped,
		m.payload, m.landedBy, m.state).Scan(&caseID)
	if err != nil {
		t.Fatalf("seeding %s: %v", email, err)
	}
	return caseID
}

// rfc822 is a received message as its mailbox stores it: the provider's own
// Received line on top, stamped `delivered`, and below it whatever the sender
// wrote, including a Received line and a Date of their choosing.
func rfc822(delivered, claimed time.Time) string {
	return "Delivered-To: rep@held.test\r\n" +
		"Received: by 2002:a05:7300:1 with SMTP id x1;\r\n        " + delivered.Format(time.RFC1123Z) + " (UTC)\r\n" +
		"X-Received: by 2002:a05:7300:2; " + time.Now().Format(time.RFC1123Z) + "\r\n" +
		"Received: from sender.elsewhere.test; " + claimed.Format(time.RFC1123Z) + "\r\n" +
		"Date: " + claimed.Format(time.RFC1123Z) + "\r\n" +
		"From: sender@elsewhere.test\r\n\r\nhello\r\n"
}

func execFile(ctx context.Context, t *testing.T, conn *pgx.Conn, path string) {
	t.Helper()
	sql, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func caseAndKind(ctx context.Context, t *testing.T, conn *pgx.Conn, caseID string) (state, kind string) {
	t.Helper()
	if err := conn.QueryRow(ctx, `
		SELECT n.state, e.kind FROM privacy_notice_case n
		  JOIN contact_acquisition_evidence e ON e.id = n.acquisition_id
		 WHERE n.id = $1`, caseID).Scan(&state, &kind); err != nil {
		t.Fatal(err)
	}
	return state, kind
}

func TestTheHeldMailUpgradeClosesDutiesForMailTheMailboxAlreadyHeld(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()
	var seat string
	if err := conn.QueryRow(ctx,
		`INSERT INTO app_user (email, display_name) VALUES ('rep@held.test', 'Rep') RETURNING id`).Scan(&seat); err != nil {
		t.Fatal(err)
	}
	connected := time.Now().AddDate(0, -1, 0)
	if _, err := conn.Exec(ctx, `
		INSERT INTO capture_connection (provider, user_id, status, created_at) VALUES ('gmail', $1, 'connected', $2)`,
		seat, connected); err != nil {
		t.Fatal(err)
	}
	old := connected.AddDate(-2, 0, 0)
	later := time.Now().Add(-time.Hour)
	const verdict = "agent:capture_counterparty_verdict"

	cases := map[string]struct{ id, wantState, wantKind string }{
		// (a) Migration 1790844232 looked at connector:* only.
		"verdict-sent": {
			historyCase(ctx, t, conn, seat, "sent@held.test", verdict, "open", old),
			"exempt_with_reason", "mailbox_history",
		},
		// (b) On Cc of mail the provider says arrived before the connection.
		"received-before": {
			receivedCase(ctx, t, conn, seat, "cc@held.test", verdict,
				receivedMail{headerDate: old, providerAt: &old}),
			"exempt_with_reason", "mailbox_history",
		},
		// (c) The same message, arriving after the connection.
		"received-after": {
			receivedCase(ctx, t, conn, seat, "late@held.test", verdict,
				receivedMail{headerDate: later, providerAt: &later}),
			"open", "unknown_legacy",
		},
		// (d) A Date header claiming the past proves nothing.
		"backdated": {
			receivedCase(ctx, t, conn, seat, "backdated@held.test", "connector:gmail",
				receivedMail{headerDate: old, providerAt: &later}),
			"open", "unknown_legacy",
		},
		// (e) A list copying everybody is not correspondence held with them.
		"bulk": {
			receivedCase(ctx, t, conn, seat, "bulk@held.test", "connector:gmail",
				receivedMail{headerDate: old, providerAt: &old, bulk: true}),
			"open", "unknown_legacy",
		},
		// Captured before the column existed: the provider's top Received stamp.
		"stamped-before": {
			receivedCase(ctx, t, conn, seat, "stamped@held.test", "connector:gmail",
				receivedMail{headerDate: old, original: rfc822(old, old)}),
			"exempt_with_reason", "mailbox_history",
		},
		// The same, stored base64 the way capture stores mail that is not UTF-8.
		"enveloped-before": {
			receivedCase(ctx, t, conn, seat, "envelope@held.test", "connector:gmail",
				receivedMail{headerDate: old, original: rfc822(old, old), enveloped: true}),
			"exempt_with_reason", "mailbox_history",
		},
		// A sender's own Received line and Date further down are not the stamp.
		"stamped-after": {
			receivedCase(ctx, t, conn, seat, "forged@held.test", "connector:gmail",
				receivedMail{headerDate: old, original: rfc822(later, old)}),
			"open", "unknown_legacy",
		},
		// An unknown a seat typed in is a claim about somewhere else.
		"typed-in": {
			receivedCase(ctx, t, conn, seat, "typed@held.test", "human:seed",
				receivedMail{headerDate: old, providerAt: &old}),
			"open", "unknown_legacy",
		},
		// A stored original only proves arrival when this seat's own Gmail or
		// Graph connection landed it: not an extension's record ...
		"extension-landed": {
			receivedCase(ctx, t, conn, seat, "unit@held.test", verdict,
				receivedMail{headerDate: old, original: rfc822(old, old), landedBy: "connector:unit"}),
			"open", "unknown_legacy",
		},
		// ... and not a copy another seat's mailbox delivered.
		"other-seat-landed": {
			receivedCase(ctx, t, conn, seat, "theirs@held.test", verdict,
				receivedMail{
					headerDate: old, original: rfc822(old, old),
					landedBy: "connector:gmail:00000000-0000-7000-8000-000000000001",
				}),
			"open", "unknown_legacy",
		},
		// Malformed evidence is no evidence, and never stops the upgrade.
		"bad-offset": {
			receivedCase(ctx, t, conn, seat, "offset@held.test", verdict,
				receivedMail{headerDate: old, original: "Received: by x; 1 Jan 2020 00:00:00 +9999\r\n\r\nhi"}),
			"open", "unknown_legacy",
		},
		"bad-base64": {
			receivedCase(ctx, t, conn, seat, "base64@held.test", verdict,
				receivedMail{headerDate: old, payload: `{"encoding": "base64", "data": "not*base64"}`}),
			"open", "unknown_legacy",
		},
		// A blocked duty closes too, and the rollback puts its reason back.
		"blocked-before": {
			receivedCase(ctx, t, conn, seat, "blocked@held.test", verdict,
				receivedMail{headerDate: old, providerAt: &old, state: "blocked"}),
			"exempt_with_reason", "mailbox_history",
		},
		// A finished duty keeps its ground; only the acquisition changes.
		"verdict-sent-done": {
			historyCase(ctx, t, conn, seat, "done@held.test", verdict, "completed", old),
			"completed", "mailbox_history",
		},
	}

	for range 2 {
		execFile(ctx, t, conn, heldMailUp)
	}
	for name, c := range cases {
		if state, kind := caseAndKind(ctx, t, conn, c.id); state != c.wantState || kind != c.wantKind {
			t.Errorf("%s: case %q on a %q acquisition, want %q on %q", name, state, kind, c.wantState, c.wantKind)
		}
	}
	var audited int
	if err := conn.QueryRow(ctx, `
		SELECT count(*) FROM audit_log
		 WHERE entity_type = 'privacy_notice_case' AND entity_id = $1`, cases["received-before"].id).Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if audited != 1 {
		t.Errorf("the settled duty was audited %d times, want once (a second run finds nothing owed)", audited)
	}

	execFile(ctx, t, conn, heldMailDown)
	execFile(ctx, t, conn, arrivalDown)
	for _, name := range []string{"verdict-sent", "received-before", "stamped-before"} {
		if state, kind := caseAndKind(ctx, t, conn, cases[name].id); state != "open" || kind != "unknown_legacy" {
			t.Errorf("after the rollback %s is %q on %q, want open on unknown_legacy", name, state, kind)
		}
	}
	if state, kind := caseAndKind(ctx, t, conn, cases["blocked-before"].id); state != "blocked" || kind != "unknown_legacy" {
		t.Errorf("after the rollback the blocked duty is %q on %q, want blocked on unknown_legacy", state, kind)
	}
	if state, kind := caseAndKind(ctx, t, conn, cases["verdict-sent-done"].id); state != "completed" || kind != "unknown_legacy" {
		t.Errorf("after the rollback the finished duty is %q on %q, want completed on unknown_legacy", state, kind)
	}

	// Up again: the stored original still carries its stamp, and the sent mail
	// is still on record, so both close again.
	execFile(ctx, t, conn, arrivalUp)
	execFile(ctx, t, conn, heldMailUp)
	for _, name := range []string{"verdict-sent", "stamped-before"} {
		if state, _ := caseAndKind(ctx, t, conn, cases[name].id); state != "exempt_with_reason" {
			t.Errorf("after down and up %s is %q, want exempt_with_reason", name, state)
		}
	}
}

// The rollback records the duty a received-mail mailbox_history acquisition
// owes once it is unknown_legacy again, and leaves alone one that migration
// 1790844232's sent-mail rule already covers.
func TestTheHeldMailRollbackRecordsTheDutyItsRulesExcused(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()
	var seat string
	if err := conn.QueryRow(ctx,
		`INSERT INTO app_user (email, display_name) VALUES ('rep@held.test', 'Rep') RETURNING id`).Scan(&seat); err != nil {
		t.Fatal(err)
	}
	connected := time.Now().AddDate(0, -1, 0)
	if _, err := conn.Exec(ctx, `
		INSERT INTO capture_connection (provider, user_id, status, created_at) VALUES ('gmail', $1, 'connected', $2)`,
		seat, connected); err != nil {
		t.Fatal(err)
	}
	old := connected.AddDate(-2, 0, 0)
	sentCase := historyCase(ctx, t, conn, seat, "sent@held.test", "connector:gmail", "completed", old)
	var sentEvidence string
	if err := conn.QueryRow(ctx, `
		UPDATE contact_acquisition_evidence SET kind = 'mailbox_history'
		 WHERE id = (SELECT acquisition_id FROM privacy_notice_case WHERE id = $1) RETURNING id`,
		sentCase).Scan(&sentEvidence); err != nil {
		t.Fatal(err)
	}
	var receivedEvidence string
	if err := conn.QueryRow(ctx, `
		WITH c AS (
		  INSERT INTO contact (full_name, source, captured_by) VALUES ('later', 'gmail', 'connector:gmail') RETURNING id
		)
		INSERT INTO contact_acquisition_evidence (contact_id, kind, occurred_at, captured_by)
		SELECT id, 'mailbox_history', now() - interval '1 year', 'agent:capture_counterparty_verdict' FROM c
		RETURNING id`).Scan(&receivedEvidence); err != nil {
		t.Fatal(err)
	}

	execFile(ctx, t, conn, heldMailDown)
	var kind string
	var owed int
	if err := conn.QueryRow(ctx, `
		SELECT e.kind, (SELECT count(*) FROM privacy_notice_case c WHERE c.acquisition_id = e.id AND c.state = 'open')
		  FROM contact_acquisition_evidence e WHERE e.id = $1`, receivedEvidence).Scan(&kind, &owed); err != nil {
		t.Fatal(err)
	}
	if kind != "unknown_legacy" || owed != 1 {
		t.Errorf("after the rollback a received-mail history acquisition is %q with %d open duties, want unknown_legacy with 1",
			kind, owed)
	}
	if err := conn.QueryRow(ctx, `SELECT kind FROM contact_acquisition_evidence WHERE id = $1`, sentEvidence).Scan(&kind); err != nil {
		t.Fatal(err)
	}
	if kind != "mailbox_history" {
		t.Errorf("after the rollback a sent-mail history acquisition is %q, want mailbox_history (1790844232 owns it)", kind)
	}
	execFile(ctx, t, conn, heldMailUp)
}
