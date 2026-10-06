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

// historyCase seeds one captured contact with an Art. 14 case: an address the
// seat mailed at `sent`, an acquisition stamped by `capturedBy`, and a case in
// `state`. It answers the case id.
func historyCase(
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

func TestTheMailboxHistoryUpgradeClosesOnlyDutiesFromMailOlderThanTheConnection(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()
	var seat string
	if err := conn.QueryRow(ctx,
		`INSERT INTO app_user (email, display_name) VALUES ('rep@history.test', 'Rep') RETURNING id`).Scan(&seat); err != nil {
		t.Fatal(err)
	}
	connected := time.Now().AddDate(0, -1, 0)
	if _, err := conn.Exec(ctx, `
		INSERT INTO capture_connection (provider, user_id, status, created_at) VALUES ('gmail', $1, 'connected', $2)`,
		seat, connected); err != nil {
		t.Fatal(err)
	}
	old := connected.AddDate(-2, 0, 0)
	cases := map[string]struct{ id, wantState, wantKind string }{
		"history":  {historyCase(ctx, t, conn, seat, "old@history.test", "connector:gmail", "open", old), "exempt_with_reason", "mailbox_history"},
		"recent":   {historyCase(ctx, t, conn, seat, "new@history.test", "connector:gmail", "open", time.Now()), "open", "unknown_legacy"},
		"handled":  {historyCase(ctx, t, conn, seat, "done@history.test", "connector:gmail", "completed", old), "completed", "mailbox_history"},
		"typed-in": {historyCase(ctx, t, conn, seat, "typed@history.test", "human:seed", "open", old), "open", "unknown_legacy"},
	}

	sql, err := os.ReadFile("core/1790844232_mail_older_than_its_mailbox_connection_owes_no_notice.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := conn.Exec(ctx, string(sql)); err != nil {
			t.Fatal(err)
		}
	}

	for name, c := range cases {
		var state, kind string
		if err := conn.QueryRow(ctx, `
			SELECT n.state, e.kind FROM privacy_notice_case n
			  JOIN contact_acquisition_evidence e ON e.id = n.acquisition_id
			 WHERE n.id = $1`, c.id).Scan(&state, &kind); err != nil {
			t.Fatal(err)
		}
		if state != c.wantState || kind != c.wantKind {
			t.Errorf("%s: case %q on a %q acquisition, want %q on %q", name, state, kind, c.wantState, c.wantKind)
		}
	}
	assertDownRecordsEveryDuty(ctx, t, conn, cases["history"].id)

	var audited int
	if err := conn.QueryRow(ctx, `
		SELECT count(*) FROM audit_log
		 WHERE entity_type = 'privacy_notice_case' AND entity_id = $1`, cases["history"].id).Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if audited != 1 {
		t.Errorf("the settled duty was audited %d times, want once (a second run finds nothing owed)", audited)
	}
}

// assertDownRecordsEveryDuty rolls the upgrade back over a mailbox_history
// acquisition that never had a case: as unknown_legacy again it owes one, and
// the duty the upgrade closed reopens.
func assertDownRecordsEveryDuty(ctx context.Context, t *testing.T, conn *pgx.Conn, closedCase string) {
	t.Helper()
	var newEvidence string
	if err := conn.QueryRow(ctx, `
		WITH c AS (
		  INSERT INTO contact (full_name, source, captured_by) VALUES ('later', 'gmail', 'connector:gmail') RETURNING id
		)
		INSERT INTO contact_acquisition_evidence (contact_id, kind, occurred_at, captured_by)
		SELECT id, 'mailbox_history', now() - interval '1 year', 'connector:gmail' FROM c RETURNING id`).Scan(&newEvidence); err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile("core/1790844232_mail_older_than_its_mailbox_connection_owes_no_notice.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, string(down)); err != nil {
		t.Fatal(err)
	}
	var owed int
	if err := conn.QueryRow(ctx, `
		SELECT count(*) FROM privacy_notice_case WHERE acquisition_id = $1 AND state = 'open'`, newEvidence).Scan(&owed); err != nil {
		t.Fatal(err)
	}
	if owed != 1 {
		t.Errorf("after the rollback a mailbox_history acquisition with no case has %d open duties, want 1", owed)
	}
	var state string
	if err := conn.QueryRow(ctx, `SELECT state FROM privacy_notice_case WHERE id = $1`, closedCase).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "open" {
		t.Errorf("after the rollback the duty the upgrade closed is %q, want open", state)
	}
}
