// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The half of SyncNow only Postgres can answer: that the row it enqueues is one
// River accepts, of the kind the clock enqueues, for the caller's workspace —
// and that a second ask coalesces onto it rather than queueing a second tick.
//
// The unit tests hold the bounds (whose job, whose tenant, what the opts say).
// What they cannot hold is whether the insert LANDS: the args carry a kind, a
// workspace and a principal, River decides whether that row is well formed, and
// a shape it rejects would leave every "checking now" a lie.

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// syncNowJob is a real composed child kind, because attendedChildOpts reads the
// declaration table for its queue and attempt cap and a made-up kind panics
// there rather than answering.
const syncNowUnit, syncNowJob = "notes", "heartbeat"

func TestSyncNowQueuesTheUnitsOwnTickForTheCallersWorkspace(t *testing.T) {
	ctx := context.Background()
	ownerDSN, appDSN := os.Getenv("MARGINCE_TEST_DSN"), os.Getenv("MARGINCE_TEST_APP_DSN")
	if ownerDSN == "" || appDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN / MARGINCE_TEST_APP_DSN not set — run `make db-up`")
	}
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing the owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if err := testdb.Reset(ctx, owner); err != nil {
		t.Fatal(err)
	}
	pool, err := testdb.Pool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}

	ws := ids.NewV7()
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, ws); err != nil {
		t.Fatal(err)
	}
	// The agent seat the tick runs under. Without it SyncNow answers
	// ErrNoUnattendedSeat, which is the other case this file could assert and
	// the one the fleet dispatch already skips a workspace for.
	seat := ids.NewV7()
	if _, err := owner.Exec(ctx,
		`INSERT INTO app_user (id, email, display_name, is_agent)
		 VALUES ($1, $2, 'Agent', true)`, seat, "agent-"+seat.String()+"@sync.test"); err != nil {
		t.Fatal(err)
	}

	decl := extension.JobDeclaration{Unit: syncNowUnit, Job: syncNowJob}
	set := []composedJob{{decl: decl}}
	before := servedExtensionJobs()
	setComposedJobs(set)
	t.Cleanup(func() { setComposedJobs(before) })
	// The DECLARATION table as well as the served set: attendedChildOpts reads
	// the child kind's queue and attempt cap out of it, so a test that
	// registered only the served half would be asking about a kind the job
	// tier has never heard of.
	if err := jobs.RegisterComposed(composedJobSpecs(set)); err != nil {
		t.Fatalf("registering the composed job specs: %v", err)
	}
	t.Cleanup(func() {
		if err := jobs.RegisterComposed(composedJobSpecs(before)); err != nil {
			t.Errorf("restoring the composed job specs: %v", err)
		}
	})

	rt := &callRuntime{
		unit:    syncNowUnit,
		live:    true,
		callCtx: principal.WithWorkspaceID(ctx, ws),
		deps:    extensionRuntimeBinding{pool: pool},
	}

	if err := rt.SyncNow(ctx, syncNowJob); err != nil {
		t.Fatalf("asking for this unit's own tick: %v", err)
	}
	if n := queuedTicks(t, owner, decl.ChildKind(), ws); n != 1 {
		t.Fatalf("queued %d tick(s) of %s, want 1 — the ask enqueued nothing River kept", n, decl.ChildKind())
	}

	// The member presses save again. The second ask must coalesce: a check
	// already queued IS the check, and a held-down button must not become a
	// tick per press.
	if err := rt.SyncNow(ctx, syncNowJob); err != nil {
		t.Fatalf("asking a second time: %v", err)
	}
	if n := queuedTicks(t, owner, decl.ChildKind(), ws); n != 1 {
		t.Errorf("queued %d tick(s) after a second ask, want still 1 — a member holding down save "+
			"enqueues a run per press", n)
	}
}

// queuedTicks counts the rows River is holding for one kind and workspace.
func queuedTicks(t *testing.T, owner *pgx.Conn, kind string, ws ids.UUID) int {
	t.Helper()
	var n int
	if err := owner.QueryRow(context.Background(),
		`SELECT count(*) FROM river_job
		  WHERE kind = $1 AND args->>'workspace_id' = $2 AND state IN ('available','scheduled','running','retryable')`,
		kind, ws.String()).Scan(&n); err != nil {
		t.Fatalf("counting queued ticks: %v", err)
	}
	return n
}
