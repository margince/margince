// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type heldReceivedMail struct {
	headerDate time.Time  // the Date header, which the sender writes
	providerAt *time.Time // the provider's arrival time, nil when none is on record
	bulk       bool
	original   string // the stored RFC822 original, empty for none
	enveloped  bool   // stored base64 in capture's envelope, as non-UTF-8 mail is
	payload    string // a stored original given verbatim as jsonb, overriding original
	landedBy   string // the activity's captured_by; empty is this seat's Gmail connection
	state      string // the case's state; empty is open
}

// heldReceivedCase seeds one captured contact that only ever appeared on the Cc
// line of a message the seat's mailbox received, with an open Art. 14 case on
// an unknown_legacy acquisition stamped by `capturedBy`. It answers the case id.
func heldReceivedCase(
	ctx context.Context, t *testing.T, conn *pgx.Conn,
	seat, email, capturedBy string, m heldReceivedMail,
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

// heldRFC822 puts the provider arrival above the sender-controlled headers.
func heldRFC822(delivered, claimed time.Time) string {
	return "Delivered-To: rep@held.test\r\n" +
		"Received: by 2002:a05:7300:1 with SMTP id x1;\r\n        " + delivered.Format(time.RFC1123Z) + " (UTC)\r\n" +
		"X-Received: by 2002:a05:7300:2; " + delivered.Add(time.Hour).Format(time.RFC1123Z) + "\r\n" +
		"Received: from sender.elsewhere.test; " + claimed.Format(time.RFC1123Z) + "\r\n" +
		"Date: " + claimed.Format(time.RFC1123Z) + "\r\n" +
		"From: sender@elsewhere.test\r\n\r\nhello\r\n"
}

func heldCaseAndKind(ctx context.Context, t *testing.T, conn *pgx.Conn, caseID string) (state, kind string) {
	t.Helper()
	if err := conn.QueryRow(ctx, `
		SELECT n.state, e.kind FROM privacy_notice_case n
		  JOIN contact_acquisition_evidence e ON e.id = n.acquisition_id
		 WHERE n.id = $1`, caseID).Scan(&state, &kind); err != nil {
		t.Fatal(err)
	}
	return state, kind
}

func heldHistoryCase(
	ctx context.Context, t *testing.T, conn *pgx.Conn,
	seat, email, capturedBy, state string, sent time.Time,
) string {
	t.Helper()
	var caseID string
	err := conn.QueryRow(ctx, `
WITH c AS (
  INSERT INTO contact (full_name, source, captured_by) VALUES ($2, 'gmail', 'connector:gmail') RETURNING id
), ce AS (
  INSERT INTO contact_email (contact_id, email, source, captured_by)
  SELECT id, $2, 'gmail', 'connector:gmail' FROM c RETURNING contact_id
), a AS (
  INSERT INTO activity (kind, subject, direction, occurred_at, source_system, source_id, source, captured_by,
                        counterparty_email, counterparty_outbound_attested)
  VALUES ('email', 'hi', 'outbound', $4, 'gmail', $2, 'gmail:seed', 'connector:gmail', $2, true) RETURNING id
), p AS (
  INSERT INTO activity_participant (activity_id, role, address, user_id)
  SELECT id, 'to', $2, NULL::uuid FROM a UNION ALL SELECT id, 'from', NULL, $1::uuid FROM a
  RETURNING activity_id
), ci AS (
  INSERT INTO capture_import (activity_id, user_id) SELECT id, $1 FROM a RETURNING id
), e AS (
  INSERT INTO contact_acquisition_evidence (contact_id, kind, occurred_at, captured_by)
  SELECT id, 'unknown_legacy', $4, $3 FROM c RETURNING id, contact_id
)
INSERT INTO privacy_notice_case (contact_id, acquisition_id, rule, due_at, state, completed_at)
SELECT e.contact_id, e.id, 'art14', $4::timestamptz + interval '1 month', $5,
       CASE WHEN $5 = 'completed' THEN now() END
  FROM e RETURNING id`,
		seat, email, capturedBy, sent, state).Scan(&caseID)
	if err != nil {
		t.Fatalf("seeding %s: %v", email, err)
	}
	return caseID
}

func TestHeldMailTaskClosesOnlyProvenHistoryAndReplaysSafely(t *testing.T) {
	e := integration.Setup(t)
	conn := integration.OwnerConn(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	var seat string
	if err := conn.QueryRow(ctx,
		`INSERT INTO app_user (email, display_name) VALUES ('rep@held.test', 'Rep') RETURNING id`).Scan(&seat); err != nil {
		t.Fatal(err)
	}
	connected := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := conn.Exec(ctx, `
		INSERT INTO capture_connection (provider, user_id, status, created_at) VALUES ('gmail', $1, 'connected', $2)`,
		seat, connected); err != nil {
		t.Fatal(err)
	}
	cases := heldMailCases(ctx, t, conn, seat, connected)

	tasks := dataTaskCatalog(func() time.Time { return connected.Add(time.Hour) })
	for i := range tasks[0].Steps {
		tasks[0].Steps[i].Limit = 2
		tasks[0].Steps[i].MinLimit = 1
	}
	engine, err := datatask.New(e.Pool, tasks, "test", func() time.Time { return connected.Add(time.Hour) }, func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	for batch := range 100 {
		if _, err := engine.Execute(ctx); err != nil {
			t.Fatal(err)
		}
		progress, err := engine.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if progress[len(progress)-1].Status == datatask.Done {
			break
		}
		if batch == 99 {
			t.Fatalf("correction did not finish: %+v", progress)
		}
	}
	assertHeldMailReplay(ctx, t, e, tasks)
	for name, c := range cases {
		if state, kind := heldCaseAndKind(ctx, t, conn, c.id); state != c.wantState || kind != c.wantKind {
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
}

func TestHeldMailDomainAndAuditRollBackWithItsProgress(t *testing.T) {
	e := integration.Setup(t)
	conn := integration.OwnerConn(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	connected := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := conn.Exec(ctx, `INSERT INTO capture_connection(provider,user_id,status,created_at) VALUES('gmail',@user,'connected',@at)`, pgx.NamedArgs{"user": e.Rep1, "at": connected}); err != nil {
		t.Fatal(err)
	}
	id := heldHistoryCase(ctx, t, conn, e.Rep1.String(), "rollback@held.test", "connector:gmail", "open", connected.Add(-time.Hour))
	now := connected.Add(time.Hour)
	tasks := dataTaskCatalog(func() time.Time { return now })
	original := tasks[0].Steps[1].Batch
	tasks[0].Steps[1].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := original(ctx, tx, in)
		if err != nil {
			return out, err
		}
		return out, errors.New("transaction interrupted after domain writes")
	}
	engine, err := datatask.New(e.Pool, tasks, "test", func() time.Time { return now }, dataTaskFailureNotice(e.Pool))
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := engine.Execute(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if state, kind := heldCaseAndKind(ctx, t, conn, id); state != "open" || kind != "unknown_legacy" {
		t.Fatalf("partial domain write: %s/%s", state, kind)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type IN ('privacy_notice_case','contact_acquisition_evidence')`); got != 0 {
		t.Fatalf("audit escaped rollback: %d", got)
	}
	progress, err := engine.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if progress[1].Batches != 0 || progress[1].ConsecutiveFailures != 1 {
		t.Fatalf("partial progress: %+v", progress[1])
	}
	now = now.Add(time.Hour)
	resumed, err := newDataTaskEngine(e.Pool, "test", func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resumed.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	if state, kind := heldCaseAndKind(ctx, t, conn, id); state != "exempt_with_reason" || kind != "mailbox_history" {
		t.Fatalf("retry failed: %s/%s", state, kind)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type IN ('privacy_notice_case','contact_acquisition_evidence')`); got != 2 {
		t.Fatalf("committed audit rows=%d", got)
	}
}

func assertHeldMailReplay(ctx context.Context, t *testing.T, e *integration.Env, tasks []datatask.Task) {
	t.Helper()
	for _, step := range tasks[0].Steps {
		in := datatask.BatchIn{Limit: 2, RunID: ids.NewV7()}
		for {
			var out datatask.BatchOut
			err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error { var err error; out, err = step.Batch(ctx, tx, in); return err })
			if err != nil {
				t.Fatal(err)
			}
			if out.Changed != 0 {
				t.Fatalf("replay changed %d records", out.Changed)
			}
			if out.Done {
				break
			}
			in.Cursor = out.Cursor
		}
	}
}

type heldMailOutcome struct{ id, wantState, wantKind string }

func heldMailCases(ctx context.Context, t *testing.T, conn *pgx.Conn, seat string, connected time.Time) map[string]heldMailOutcome {
	t.Helper()
	old := connected.AddDate(-2, 0, 0)
	later := connected.Add(time.Hour)
	const verdict = "agent:capture_counterparty_verdict"

	return map[string]heldMailOutcome{
		// Verdict-derived acquisitions use the same sent-history evidence.
		"verdict-sent": {
			heldHistoryCase(ctx, t, conn, seat, "sent@held.test", verdict, "open", old),
			"exempt_with_reason", "mailbox_history",
		},
		// (b) On Cc of mail the provider says arrived before the connection.
		"received-before": {
			heldReceivedCase(ctx, t, conn, seat, "cc@held.test", verdict,
				heldReceivedMail{headerDate: old, providerAt: &old}),
			"exempt_with_reason", "mailbox_history",
		},
		// (c) The same message, arriving after the connection.
		"received-after": {
			heldReceivedCase(ctx, t, conn, seat, "late@held.test", verdict,
				heldReceivedMail{headerDate: later, providerAt: &later}),
			"open", "unknown_legacy",
		},
		// (d) A Date header claiming the past proves nothing.
		"backdated": {
			heldReceivedCase(ctx, t, conn, seat, "backdated@held.test", "connector:gmail",
				heldReceivedMail{headerDate: old, providerAt: &later}),
			"open", "unknown_legacy",
		},
		// (e) A list copying everybody is not correspondence held with them.
		"bulk": {
			heldReceivedCase(ctx, t, conn, seat, "bulk@held.test", "connector:gmail",
				heldReceivedMail{headerDate: old, providerAt: &old, bulk: true}),
			"open", "unknown_legacy",
		},
		// Captured before the column existed: the provider's top Received stamp.
		"stamped-before": {
			heldReceivedCase(ctx, t, conn, seat, "stamped@held.test", "connector:gmail",
				heldReceivedMail{headerDate: old, original: heldRFC822(old, old)}),
			"exempt_with_reason", "mailbox_history",
		},
		// The same, stored base64 the way capture stores mail that is not UTF-8.
		"enveloped-before": {
			heldReceivedCase(ctx, t, conn, seat, "envelope@held.test", "connector:gmail",
				heldReceivedMail{headerDate: old, original: heldRFC822(old, old), enveloped: true}),
			"exempt_with_reason", "mailbox_history",
		},
		// A sender's own Received line and Date further down are not the stamp.
		"stamped-after": {
			heldReceivedCase(ctx, t, conn, seat, "forged@held.test", "connector:gmail",
				heldReceivedMail{headerDate: old, original: heldRFC822(later, old)}),
			"open", "unknown_legacy",
		},
		// An unknown a seat typed in is a claim about somewhere else.
		"typed-in": {
			heldReceivedCase(ctx, t, conn, seat, "typed@held.test", "human:seed",
				heldReceivedMail{headerDate: old, providerAt: &old}),
			"open", "unknown_legacy",
		},
		// A stored original only proves arrival when this seat's own Gmail or
		// Graph connection landed it: not an extension's record ...
		"extension-landed": {
			heldReceivedCase(ctx, t, conn, seat, "unit@held.test", verdict,
				heldReceivedMail{headerDate: old, original: heldRFC822(old, old), landedBy: "connector:unit"}),
			"open", "unknown_legacy",
		},
		// ... and not a copy another seat's mailbox delivered.
		"other-seat-landed": {
			heldReceivedCase(ctx, t, conn, seat, "theirs@held.test", verdict,
				heldReceivedMail{
					headerDate: old, original: heldRFC822(old, old),
					landedBy: "connector:gmail:00000000-0000-7000-8000-000000000001",
				}),
			"open", "unknown_legacy",
		},
		// Malformed evidence is no evidence, and never stops the upgrade.
		"bad-offset": {
			heldReceivedCase(ctx, t, conn, seat, "offset@held.test", verdict,
				heldReceivedMail{headerDate: old, original: "Received: by x; 1 Jan 2020 00:00:00 +9999\r\n\r\nhi"}),
			"open", "unknown_legacy",
		},
		"malformed-first": {
			heldReceivedCase(ctx, t, conn, seat, "first@held.test", verdict, heldReceivedMail{headerDate: old, original: "Received: malformed\r\n" + heldRFC822(old, old)}),
			"open", "unknown_legacy",
		},
		"truncated-header": {
			heldReceivedCase(ctx, t, conn, seat, "truncated@held.test", verdict, heldReceivedMail{headerDate: old, original: "X-Padding: " + strings.Repeat("a", 65536) + "\r\n" + heldRFC822(old, old)}),
			"open", "unknown_legacy",
		},
		"truncated-envelope": {
			heldReceivedCase(ctx, t, conn, seat, "truncated-envelope@held.test", verdict, heldReceivedMail{headerDate: old, original: "X-Padding: " + strings.Repeat("a", 65536) + "\r\n" + heldRFC822(old, old), enveloped: true}),
			"open", "unknown_legacy",
		},
		"bad-base64": {
			heldReceivedCase(ctx, t, conn, seat, "base64@held.test", verdict,
				heldReceivedMail{headerDate: old, payload: `{"encoding": "base64", "data": "not*base64"}`}),
			"open", "unknown_legacy",
		},
		// Outstanding duties close regardless of delivery or assignment state.
		"blocked-before": {
			heldReceivedCase(ctx, t, conn, seat, "blocked@held.test", verdict,
				heldReceivedMail{headerDate: old, providerAt: &old, state: "blocked"}),
			"exempt_with_reason", "mailbox_history",
		},
		"assigned-before": {
			heldReceivedCase(ctx, t, conn, seat, "assigned@held.test", verdict,
				heldReceivedMail{headerDate: old, providerAt: &old, state: "assigned"}),
			"exempt_with_reason", "mailbox_history",
		},
		"delivery-failed-before": {
			heldReceivedCase(ctx, t, conn, seat, "delivery-failed@held.test", verdict,
				heldReceivedMail{headerDate: old, providerAt: &old, state: "delivery_failed"}),
			"exempt_with_reason", "mailbox_history",
		},
		// A finished duty keeps its ground; only the acquisition changes.
		"verdict-sent-done": {
			heldHistoryCase(ctx, t, conn, seat, "done@held.test", verdict, "completed", old),
			"completed", "mailbox_history",
		},
	}
}
