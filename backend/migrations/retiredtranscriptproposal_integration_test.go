// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"os"
	"testing"
)

func TestTheRetiredTranscriptProposalUpgradeClosesOnlyItsPendingProposals(t *testing.T) {
	dsn, _ := dsns(t)
	conn := connect(t, dsn)
	headSchema(t, conn)
	ctx := context.Background()
	seed, err := conn.Exec(ctx, `INSERT INTO approval(kind,status,proposed_by,proposed_change,diff_hash,expires_at,decided_at)
 VALUES ('transcript_proposal','pending','agent:test','{}','t1',now() + interval '1 day',NULL),
        ('transcript_proposal','pending','agent:test','{}','t2',now() + interval '2 days',NULL),
        ('transcript_proposal','approved','agent:test','{}','t3',now() + interval '1 day',now()),
        ('commitment_task','pending','agent:test','{}','t4',now() + interval '1 day',NULL)`)
	if err != nil || seed.RowsAffected() != 4 {
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
	// Closed means the window is over: the expiry sweep settles the row with
	// its own audit and event, and no decision is taken on it meanwhile.
	for diff, closed := range map[string]bool{"t1": true, "t2": true, "t3": false, "t4": false} {
		var lapsed bool
		if err := conn.QueryRow(ctx, `SELECT expires_at <= now() FROM approval WHERE diff_hash = $1`, diff).Scan(&lapsed); err != nil {
			t.Fatal(err)
		}
		if lapsed != closed {
			t.Errorf("proposal %s window closed = %v, want %v", diff, lapsed, closed)
		}
	}
	var audited int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM audit_log
 WHERE entity_type='approval' AND action='update'
   AND entity_id IN (SELECT id FROM approval WHERE diff_hash IN ('t1','t2'))`).Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if audited != 2 {
		t.Fatalf("closing two windows was audited %d times, want once each (a second run finds nothing open)", audited)
	}
}
