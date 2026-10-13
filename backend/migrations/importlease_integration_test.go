// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
)

const importLeaseChecksVersion = "20261013130001"

// leaseColumns lists the columns the lease migration adds, so the down test can
// prove each one leaves again.
var leaseColumns = []string{
	"job_attempt", "job_status", "claimed_attempt", "heartbeat_at", "progress_at", "started_at",
	"finished_at", "phase", "phase_object", "progress", "resume_count", "actor", "attempt_correlation_id",
}

func TestAnOldShapeInsertStartsTheLeaseAtAttemptZero(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	id := seedLegacyRun(t, conn, legacyRun{status: "running", sourceRef: "ws/import/lease"})

	var attempt, resumes int
	var jobStatus, phase *string
	var claimed *int
	if err := conn.QueryRow(context.Background(), `
		SELECT job_attempt, resume_count, job_status, phase, claimed_attempt
		  FROM import_run WHERE id::text = $1`, id).Scan(&attempt, &resumes, &jobStatus, &phase, &claimed); err != nil {
		t.Fatal(err)
	}
	// Attempt zero is what marks a row old code moved, so the sweep never re-queues it.
	if attempt != 0 || resumes != 0 || jobStatus != nil || phase != nil || claimed != nil {
		t.Errorf("an old-shape insert reads attempt %d, resumes %d, job_status %v, phase %v, claimed %v; "+
			"want attempt 0, no resumes and no lease", attempt, resumes, jobStatus, phase, claimed)
	}
}

func TestThePhaseCheckRefusesAnUnknownPhase(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	id := seedLegacyRun(t, conn, legacyRun{status: "running", sourceRef: "ws/import/phase"})
	ctx := context.Background()

	if _, err := conn.Exec(ctx, `UPDATE import_run SET phase = 'importing' WHERE id::text = $1`, id); err != nil {
		t.Fatalf("a known phase was refused: %v", err)
	}
	_, err := conn.Exec(ctx, `UPDATE import_run SET phase = 'queued' WHERE id::text = $1`, id)
	if err == nil || !strings.Contains(err.Error(), "import_run_phase_check") {
		t.Fatalf("phase 'queued' = %v, want the phase check to refuse it: queued is derived at read, never stored", err)
	}
	var validated bool
	if err := conn.QueryRow(ctx,
		`SELECT convalidated FROM pg_constraint WHERE conname = 'import_run_phase_check'`).Scan(&validated); err != nil {
		t.Fatal(err)
	}
	if !validated {
		t.Error("the phase check is still NOT VALID at head; the second migration validates it")
	}
}

func TestTheLeaseDownMigrationDropsEveryLeaseColumn(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	seedLegacyRun(t, conn, legacyRun{status: "complete", sourceRef: "ws/import/kept"})
	ctx := context.Background()

	rewound := namespaceThroughVersion(t, customNamespace(t), importLeaseChecksVersion)
	if _, err := dbmigrate.Down(ctx, conn, rewound, 2); err != nil {
		t.Fatalf("down: %v", err)
	}
	var left []string
	rows, err := conn.Query(ctx, `
		SELECT column_name FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = 'import_run' AND column_name = ANY($1)`, leaseColumns)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		left = append(left, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Errorf("after down import_run still has %v", left)
	}
	var runs int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM import_run`).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if runs != 1 {
		t.Errorf("after down %d runs remain, want the one complete run: the lease owns no rows", runs)
	}
	replayImportSession(t, conn)
}
