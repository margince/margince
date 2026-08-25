// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The cg:org-auto-enrich trigger: an organization event queues the workspace's
// auto-enrich pass NOW, a burst of them collapses onto one queued pass, and
// traffic that is not its business queues nothing.

import (
	"context"
	"log/slog"
	"testing"

	"github.com/gradionhq/margince/backend/internal/compose/integration"
	"github.com/gradionhq/margince/backend/internal/platform/jobs"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
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

	// An archive can only make organizations LESS due, so it must queue
	// nothing — and it must answer nil, because the group carries every
	// organization event and a consumer that errored on one would wedge the
	// stream for the rest.
	if err := trigger.HandleEvent(ctx, envelopeFor(e.WS, "organization.archived", "organization", ids.NewV7())); err != nil {
		t.Errorf("an archive event errored: %v", err)
	}
	if n := countJobsOfKind(ctx, t, e.Pool, kind); n != 0 {
		t.Fatalf("an archive event queued %d enrich pass(es), want none", n)
	}

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
	if err := e.Pool.QueryRow(ctx,
		`SELECT coalesce(args->>'workspace_id', '') FROM river_job WHERE kind = $1`, kind,
	).Scan(&workspace); err != nil {
		t.Fatalf("reading the queued pass: %v", err)
	}
	if workspace != e.WS.String() {
		t.Errorf("the queued pass names workspace %q, want the installation's %q", workspace, e.WS)
	}

	// A second event queues its own pass rather than deduping onto the first:
	// a pass already claimed by a worker may have listed the due organizations
	// before this event's rows landed, so dropping the event could strand
	// exactly the company it announced. Redundancy is the cheap side — the
	// pass's own gates make a repeat over the same organization a no-op.
	if err := trigger.HandleEvent(ctx, envelopeFor(e.WS, "organization.updated", "organization", ids.NewV7())); err != nil {
		t.Fatalf("organization.updated: %v", err)
	}
	if n := countJobsOfKind(ctx, t, e.Pool, kind); n != 2 {
		t.Fatalf("two organization events left %d queued enrich pass(es), want one each", n)
	}
}
