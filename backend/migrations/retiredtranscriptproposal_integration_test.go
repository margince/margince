// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"os"
	"testing"
)

func TestTheRetiredTranscriptProposalUpgradeExpiresOnlyItsPendingProposals(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()
	seed, err := conn.Exec(ctx, `INSERT INTO approval(kind,status,proposed_by,proposed_change,diff_hash,expires_at,decided_at)
 VALUES ('transcript_proposal','pending','agent:test','{}','t1',now() + interval '1 day',NULL),
        ('transcript_proposal','approved','agent:test','{}','t2',now() + interval '1 day',now()),
        ('commitment_task','pending','agent:test','{}','t3',now() + interval '1 day',NULL)`)
	if err != nil || seed.RowsAffected() != 3 {
		t.Fatalf("seeding approvals: rows=%d err=%v", seed.RowsAffected(), err)
	}
	sql, err := os.ReadFile("core/1791170188_a_retired_transcript_proposal_leaves_no_orphans.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := conn.Exec(ctx, string(sql)); err != nil {
			t.Fatal(err)
		}
	}
	for diff, want := range map[string]string{"t1": "expired", "t2": "approved", "t3": "pending"} {
		var status string
		if err := conn.QueryRow(ctx, `SELECT status FROM approval WHERE diff_hash = $1`, diff).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != want {
			t.Errorf("proposal %s is %q, want %q", diff, status, want)
		}
	}
	var audited int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM audit_log
 WHERE entity_type='approval' AND action='expire'
   AND entity_id IN (SELECT id FROM approval WHERE diff_hash='t1')`).Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if audited != 1 {
		t.Fatalf("the expiry was audited %d times, want once (a second run finds nothing pending)", audited)
	}
}
