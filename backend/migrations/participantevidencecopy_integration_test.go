// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// participantEvidence is what one activity's participant rows carry of it.
type participantEvidence struct {
	rows    int
	agree   bool
	version int64
}

func readParticipantEvidence(t *testing.T, conn *pgx.Conn, activity string) participantEvidence {
	t.Helper()
	var got participantEvidence
	if err := conn.QueryRow(context.Background(), `
		SELECT count(*), a.domain_evidence_version, coalesce(bool_and(
		         p.domain_evidence_at = a.occurred_at AND p.domain_evidence_precision = a.occurred_precision
		         AND p.domain_evidence_visible = (a.archived_at IS NULL AND a.restricted_at IS NULL)
		         AND p.domain_evidence_parent_version = a.domain_evidence_version), false)
		  FROM activity a JOIN activity_participant p ON p.activity_id = a.id
		 WHERE a.id = $1 GROUP BY a.id`, activity).Scan(&got.rows, &got.version, &got.agree); err != nil {
		t.Fatalf("reading the participant evidence of %s: %v", activity, err)
	}
	return got
}

// One statement changes several activities here, because a per-row copy that
// read only the first changed row would pass a single-row test.
func TestAnActivityChangeReachesItsParticipantRows(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()
	activities := []string{
		seedEvidenceActivity(t, conn, "First"), seedEvidenceActivity(t, conn, "Second"), seedEvidenceActivity(t, conn, "Third"),
	}
	for _, activity := range activities {
		if _, err := conn.Exec(ctx, `INSERT INTO activity_participant (activity_id, role, address)
			VALUES ($1, 'from', 'sender@evidencecopy.test'), ($1, 'to', 'reader@evidencecopy.test')`, activity); err != nil {
			t.Fatalf("seeding participants: %v", err)
		}
	}
	changes := []struct {
		name, statement string
		argument        any
		version         int64
		visible         bool
	}{
		{"a text change", `UPDATE activity SET subject = $2 WHERE id = ANY($1)`, "Renamed", 0, true},
		{"a time change", `UPDATE activity SET occurred_at = $2 WHERE id = ANY($1)`, time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC), 1, true},
		{"a precision change", `UPDATE activity SET occurred_precision = $2 WHERE id = ANY($1)`, "day", 2, true},
		{"an archive", `UPDATE activity SET archived_at = $2 WHERE id = ANY($1)`, time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC), 3, false},
		{"a restore", `UPDATE activity SET archived_at = $2 WHERE id = ANY($1)`, nil, 4, true},
	}
	for _, change := range changes {
		if _, err := conn.Exec(ctx, change.statement, activities, change.argument); err != nil {
			t.Fatalf("%s: %v", change.name, err)
		}
		for _, activity := range activities {
			got := readParticipantEvidence(t, conn, activity)
			if got.rows != 2 || !got.agree || got.version != change.version {
				t.Errorf("after %s: %d participant rows, agreeing=%t, at version %d, want 2 agreeing rows at version %d",
					change.name, got.rows, got.agree, got.version, change.version)
			}
			var visible bool
			if err := conn.QueryRow(ctx, `SELECT bool_and(domain_evidence_visible) FROM activity_participant
				WHERE activity_id = $1`, activity).Scan(&visible); err != nil {
				t.Fatal(err)
			}
			if visible != change.visible {
				t.Errorf("after %s the participant rows read visible=%t, want %t", change.name, visible, change.visible)
			}
		}
	}
}

func TestAParticipantRowStillNeedsItsActivity(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()
	_, err := conn.Exec(ctx, `INSERT INTO activity_participant (activity_id, role, address)
		VALUES (uuidv7(), 'to', 'orphan@evidencecopy.test')`)
	var refused *pgconn.PgError
	if !errors.As(err, &refused) || refused.Code != "23503" {
		t.Errorf("a participant row naming no stored activity was accepted: %v", err)
	}
	activity := seedEvidenceActivity(t, conn, "Deleted with its participants")
	if _, err := conn.Exec(ctx, `INSERT INTO activity_participant (activity_id, role, address)
		VALUES ($1, 'to', 'reader@evidencecopy.test')`, activity); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `DELETE FROM activity WHERE id = $1`, activity); err != nil {
		t.Fatalf("deleting an activity with a participant row: %v", err)
	}
	var left int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM activity_participant WHERE activity_id = $1`, activity).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("a deleted activity left %d participant rows", left)
	}
}

// The row lock must follow the advisory locks the other triggers take.
// A participant insert takes those first, so the other order can deadlock.
func TestTheStampWaitIsTheLastTriggerBeforeAnActivityUpdate(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	var last string
	var count int
	// The two `tgtype` bits select row triggers that run before an update, which fire in name order.
	if err := conn.QueryRow(context.Background(), `
		SELECT max(tgname COLLATE "C"), count(*) FROM pg_trigger
		 WHERE tgrelid = 'activity'::regclass AND NOT tgisinternal
		   AND tgtype & 2 = 2 AND tgtype & 16 = 16`).Scan(&last, &count); err != nil {
		t.Fatal(err)
	}
	if count < 5 {
		t.Fatalf("read %d triggers before an activity update, fewer than the 5 the schema holds", count)
	}
	if last != "wait_for_participant_stamps" {
		t.Errorf("%s fires after wait_for_participant_stamps, so the row lock precedes whatever lock it takes", last)
	}
}
