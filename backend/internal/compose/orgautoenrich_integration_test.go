// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The cg:org-auto-enrich trigger against the real queue: an organization
// event queues the workspace's auto-enrich pass NOW, addressed to the right
// workspace, and a burst collapses onto the one pass already queued. The
// refusal arm — events the trigger ignores — is the unit lane's
// (orgautoenrich_test.go), where a nil pool proves the refusal precedes the
// query.

import (
	"context"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnOrganizationEventQueuesTheEnrichPassNow(t *testing.T) {
	e := integration.Setup(t)
	inserter, err := jobs.NewInserter(e.Pool, slog.Default())
	if err != nil {
		t.Fatalf("NewInserter: %v", err)
	}
	trigger := NewOrgAutoEnrichTrigger(e.Pool, inserter, slog.Default())
	ctx := context.Background()
	kind := CaptureAutoEnrichWorkspaceArgs{}.Kind()

	// A create queues exactly one pass, addressed to the installation's own
	// workspace — the envelope carries no tenant, so naming the wrong one
	// here would run the pass against nobody's data and read as a no-op.
	if err := trigger.HandleEvent(ctx, envelopeFor(e.WS, "organization.created", "organization", ids.NewV7())); err != nil {
		t.Fatalf("organization.created: %v", err)
	}
	if n := countJobsOfKind(ctx, t, e.Pool, kind); n != 1 {
		t.Fatalf("organization.created queued %d enrich pass(es), want exactly 1", n)
	}
	var workspace string
	if err := e.Pool.QueryRow(
		ctx,
		`SELECT coalesce(args->>'workspace_id', '') FROM river_job WHERE kind = $1`, kind,
	).Scan(&workspace); err != nil {
		t.Fatalf("reading the queued pass: %v", err)
	}
	if workspace != e.WS.String() {
		t.Errorf("the queued pass names workspace %q, want the installation's %q", workspace, e.WS)
	}

	// A burst dedupes onto the pass already queued: the uniqueness is the
	// flood bound, because any authenticated writer can emit
	// organization.updated in a loop and every event landing its own row
	// would bury the shared default queue.
	if err := trigger.HandleEvent(ctx, envelopeFor(e.WS, "organization.updated", "organization", ids.NewV7())); err != nil {
		t.Fatalf("organization.updated: %v", err)
	}
	if n := countJobsOfKind(ctx, t, e.Pool, kind); n != 1 {
		t.Fatalf("a second event left %d queued enrich pass(es), want the burst deduped onto 1", n)
	}
}
