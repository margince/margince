// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WorkspaceScoped is implemented by job args whose work belongs to exactly
// one workspace. The job row IS that workspace's pass: it succeeds or fails
// on its own, is retried on its own, and reports its own failure — none of
// which is true of a pass that loops the fleet inside one row.
//
// The accessor is WorkspaceID rather than a bare field because Go forbids a
// method and a field of the same name, so implementations hold the value in a
// `Workspace ids.UUID` field, wired as `json:"workspace_id"` — one spelling on
// every kind, held there by
// TestEveryWorkspaceScopedArgsSpellsItsWorkspaceKeyTheSameWay. A kind that
// spelled it differently would be invisible to `args->>'workspace_id'`, and a
// per-workspace read of river_job would report it as no work at all rather
// than as work the query cannot see.
//
// That read is exact in both directions: a null in that column means a
// dispatcher, because a job that does tenant work declares its workspace.
type WorkspaceScoped interface {
	river.JobArgs
	WorkspaceID() ids.UUID
}

// FleetWide is implemented by the args of a job that answers for the WHOLE
// installation rather than for one tenant: it reaches every workspace, either
// by enqueuing one WorkspaceScoped job per workspace or — since ADR-0103
// collapsed the workspace dispatchers — by walking them itself.
//
// It no longer implies "does no tenant write". A collapsed pass writes for
// every workspace it walks, which is the point of it; what the marker still
// says is that the row owns no single tenant, so nothing may read a workspace
// off its args.
//
// The marker method is empty on purpose — it is a declaration, not
// behaviour, and it is what the G1 gate reads.
type FleetWide interface {
	river.JobArgs
	FleetWide()
}
