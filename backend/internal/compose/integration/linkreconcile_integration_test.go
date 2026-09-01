// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The repair sweep, driven through the WORKER rather than the store.
//
// The store-level tests call the repair on a context the test built, so they
// proved the SQL and nothing about the context the job actually runs under. That
// gap shipped: the worker bound a workspace and a principal but no correlation
// id, storekit.EmitEvent refuses to publish without one, and every contact
// failed on its own event. The sweep retried three times, was discarded, and the
// backlog it exists to clear sat untouched — looking exactly like a job that had
// not run yet.
//
// So this drives the real worker over a real database, and asserts the repair
// LANDED. A test that only checked the job returned nil would have passed while
// the rows stayed broken.

import (
	"context"
	"testing"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// sweepPersonPerms may create the contact this fixture strands mail under.
var sweepPersonPerms = principal.Permissions{
	Objects:  map[string]principal.ObjectGrant{"person": {Create: true, Read: true}},
	RowScope: principal.RowScopeAll,
}

func TestTheRepairSweepActuallyRepairsWhenDrivenAsTheJob(t *testing.T) {
	e := Setup(t)
	store := people.NewStore(e.DB())

	// A contact, and mail captured under their address that no link reaches —
	// the shape a backfill leaves when the person arrives mid-run.
	person, err := store.EnsurePersonByEmail(e.As(e.Rep1, []ids.UUID{e.Team1}, sweepPersonPerms),
		"Stranded Sender", "stranded@sweep.test", "manual")
	if err != nil {
		t.Fatalf("seeding the contact: %v", err)
	}
	activity := ids.NewV7()
	e.WsExec(t, `
		INSERT INTO activity (id, kind, subject, direction, counterparty_email,
		                      source_system, source_id, source, captured_by)
		VALUES ($1, 'email', 'hello', 'inbound', 'stranded@sweep.test',
		        'gmail', $2, 'gmail:seed', 'connector:gmail')`, activity, activity.String())
	e.WsExec(t, `
		INSERT INTO activity_participant (id, activity_id, address, role)
		VALUES ($1, $2, 'stranded@sweep.test', 'from')`, ids.NewV7(), activity)

	// Drive the worker exactly as River does.
	worker := compose.NewLinkReconcileWorkspaceWorkerForTest(e.Pool, store)
	if err := worker.Work(context.Background(), &river.Job[compose.LinkReconcileWorkspaceArgs]{
		Args: compose.LinkReconcileWorkspaceArgs{Workspace: e.WS},
	}); err != nil {
		t.Fatalf("the sweep failed: %v", err)
	}

	linked := e.WsCount(t, `SELECT count(*) FROM activity_link
	                         WHERE activity_id = $1 AND entity_type = 'person' AND person_id = $2`,
		activity, person) > 0
	named := e.WsCount(t, `SELECT count(*) FROM activity_participant
	                        WHERE activity_id = $1 AND person_id = $2`, activity, person) > 0
	if !linked || !named {
		t.Errorf("after the sweep: linked=%v named=%v, want both — the job returning cleanly is not "+
			"the repair landing, and that difference is what let a sweep that repaired nothing look healthy",
			linked, named)
	}
}
