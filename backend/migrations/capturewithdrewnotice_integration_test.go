// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

const captureWithdrewUp = "datafix/2026-10-03-3_a_contact_capture_withdrew_owes_no_notice.up.sql"

// archivedWithDuty seeds a capture-made contact archived by `archiver`, with an
// open Art. 14 case on it, and answers the case id.
func archivedWithDuty(ctx context.Context, t *testing.T, conn *pgx.Conn, name, archiver string) string {
	t.Helper()
	var caseID string
	err := conn.QueryRow(ctx, `
WITH c AS (
  INSERT INTO contact (full_name, source, captured_by, archived_at)
  VALUES ($1, 'capture_counterparty_verdict', 'agent:capture_counterparty_verdict', now()) RETURNING id
), al AS (
  INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id)
  SELECT CASE WHEN starts_with($2, 'human:') THEN 'human' ELSE 'agent' END, $2, 'archive', 'contact', id FROM c
), e AS (
  INSERT INTO contact_acquisition_evidence (contact_id, kind, occurred_at, captured_by)
  SELECT id, 'unknown_legacy', now(), 'agent:capture_counterparty_verdict' FROM c RETURNING id, contact_id
)
INSERT INTO privacy_notice_case (contact_id, acquisition_id, rule, due_at, state)
SELECT contact_id, id, 'art14', now() + interval '1 month', 'open' FROM e RETURNING id`,
		name, archiver).Scan(&caseID)
	if err != nil {
		t.Fatal(err)
	}
	return caseID
}

func TestAContactAVerdictWithdrewEndsItsNoticeDuty(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()

	withdrawn := archivedWithDuty(ctx, t, conn, "Withdrawn", "agent:capture_confidentiality_verdict")
	// A human's archive is a decision about the record, not a verdict that it
	// was never business data, and its duty stays.
	archivedByHand := archivedWithDuty(ctx, t, conn, "Archived by hand", "human:01a07544-e5ac-7844-8569-1f5df68e962c")

	// Withdrawn by a verdict, restored, then archived again by a person: the
	// current archive is the person's.
	rearchived := archivedWithDuty(ctx, t, conn, "Re-archived", "agent:capture_confidentiality_verdict")
	if _, err := conn.Exec(ctx, `
		INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, occurred_at)
		SELECT 'human', 'human:01a07544-e5ac-7844-8569-1f5df68e962c', action, 'contact', n.contact_id,
		       now() + make_interval(secs => ord)
		  FROM privacy_notice_case n, unnest(ARRAY['restore', 'archive']) WITH ORDINALITY AS x(action, ord)
		 WHERE n.id = $1`, rearchived); err != nil {
		t.Fatal(err)
	}

	execFile(ctx, t, conn, captureWithdrewUp)
	execFile(ctx, t, conn, captureWithdrewUp)

	for id, want := range map[string]string{withdrawn: "exempt_with_reason", archivedByHand: "open", rearchived: "open"} {
		var state string
		if err := conn.QueryRow(ctx, `SELECT state FROM privacy_notice_case WHERE id = $1`, id).Scan(&state); err != nil {
			t.Fatal(err)
		}
		if state != want {
			t.Errorf("case %s is %s, want %s", id, state, want)
		}
	}
}
