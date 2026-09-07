// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// A run that has stopped says WHEN it stopped, and the database is what makes
// that so.
//
// agent_run.finished_at is load-bearing: the personal activity read bounds
// settled work with `finished_at >= <local midnight>` and orders by it, because
// "settled today" is a fact about when a run FINISHED. A terminal run whose
// finished_at is NULL has left the in-flight statuses and cannot match that
// predicate, so it does not arrive late or out of order — it disappears from
// the feed.
//
// status and finished_at were independent columns and every writer was trusted
// to set them together. One did not, and the only symptom was a run silently
// missing from a reader's day (#2187). Asked of the estate rather than of the
// writers, because a census of writers is a list somebody has to keep and this
// answers for the one that has not been written yet.

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// seedRun writes one run in the given state and answers whether the estate
// accepted it. The columns are the table's own minimum: everything else has a
// default or is nullable.
func seedRun(t *testing.T, conn *pgx.Conn, status string, stamped bool) error {
	t.Helper()
	finished := "NULL"
	if stamped {
		finished = "now()"
	}
	_, err := conn.Exec(context.Background(), `
		INSERT INTO agent_run (agent_spec, goal, trigger_ref, status, finished_at)
		VALUES ('probe_spec', 'a probe', $1, $2, `+finished+`)`,
		t.Name()+status+finished, status)
	return err
}

func TestATerminalRunWithoutAFinishIsRefused(t *testing.T) {
	ownerDSN, _ := dsns(t)
	owner := connect(t, ownerDSN)
	headSchema(t, owner)

	// Every terminal status, not the one that happened to have the bug: the
	// constraint names three and a test that checked one would pass while two
	// of them stayed open.
	for _, status := range []string{"completed", "degraded", "failed"} {
		if err := seedRun(t, owner, status, false); err == nil {
			t.Errorf("the estate accepted a %s run with no finished_at — such a run has left the "+
				"in-flight statuses and cannot match the settled predicate, so it vanishes from the "+
				"feed rather than arriving late", status)
			continue
		} else if !strings.Contains(err.Error(), "agent_run_settled_shape") {
			t.Errorf("a %s run with no finished_at was refused by something else: %v", status, err)
		}
		// And the same status WITH a stamp is ordinary.
		if err := seedRun(t, owner, status, true); err != nil {
			t.Errorf("a settled %s run carrying its finish was refused: %v", status, err)
		}
	}

	// The in-flight half, which is the reason this is an implication and not
	// NOT NULL on the column: a run that has not stopped has no finish, and a
	// default would invent one.
	for _, status := range []string{"running", "awaiting_approval"} {
		err := seedRun(t, owner, status, false)
		if status == "awaiting_approval" {
			// agent_run_awaiting_shape wants an approval and a pending call,
			// which this probe does not seed — a different rule, and refusing
			// here would say nothing about the one under test.
			if err != nil && !strings.Contains(err.Error(), "agent_run_awaiting_shape") {
				t.Errorf("an %s run was refused by the settled rule: %v", status, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("a %s run with no finished_at was refused, and an unfinished run has no finish to give: %v", status, err)
		}
	}
}

// The backfill leaves no row the constraint would refuse, and stamps it with
// the truest instant the record holds.
//
// No row is expected on a tree whose writers are already fixed — the point is
// an installation that ran the old code. Driven by putting the estate back into
// that state on purpose, because a migration whose backfill was never run
// against the shape it exists for is a migration that fails on the one
// installation that needed it.
func TestTheBackfillStampsARunThatStoppedWithoutSayingWhen(t *testing.T) {
	ownerDSN, _ := dsns(t)
	owner := connect(t, ownerDSN)
	headSchema(t, owner)
	ctx := context.Background()

	// The pre-migration estate: the rule gone, and a row that could only exist
	// while it was.
	if _, err := owner.Exec(ctx, `ALTER TABLE agent_run DROP CONSTRAINT agent_run_settled_shape`); err != nil {
		t.Fatalf("removing the rule to reproduce the old estate: %v", err)
	}
	var stranded string
	if err := owner.QueryRow(ctx, `
		INSERT INTO agent_run (agent_spec, goal, trigger_ref, status, finished_at, updated_at)
		VALUES ('probe_spec', 'stopped without saying when', $1, 'failed', NULL, now() - interval '2 hours')
		RETURNING id::text`, t.Name()).Scan(&stranded); err != nil {
		t.Fatalf("seeding the stranded run: %v", err)
	}

	// The migration's own backfill, verbatim.
	if _, err := owner.Exec(ctx, `
		UPDATE agent_run
		   SET finished_at = updated_at
		 WHERE status IN ('completed', 'degraded', 'failed')
		   AND finished_at IS NULL`); err != nil {
		t.Fatalf("the backfill: %v", err)
	}

	var matchesUpdated bool
	if err := owner.QueryRow(ctx,
		`SELECT finished_at IS NOT NULL AND finished_at = updated_at FROM agent_run WHERE id::text = $1`,
		stranded).Scan(&matchesUpdated); err != nil {
		t.Fatalf("reading the backfilled run: %v", err)
	}
	if !matchesUpdated {
		t.Error("the stranded run did not take updated_at as its finish — every terminal writer stamps " +
			"updated_at in the same statement it sets the status, so for a row that reached a terminal " +
			"state the two are the same instant")
	}

	// And the rule goes back on, which is what the backfill is FOR: without it
	// this statement fails and the migration takes the installation with it.
	if _, err := owner.Exec(ctx, `
		ALTER TABLE agent_run
		    ADD CONSTRAINT agent_run_settled_shape
		    CHECK (status NOT IN ('completed', 'degraded', 'failed') OR finished_at IS NOT NULL)`); err != nil {
		t.Fatalf("the rule could not be added after the backfill, so the migration would fail on an "+
			"installation that ran the old code: %v", err)
	}
}
