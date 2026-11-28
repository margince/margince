// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river/rivertype"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// bindInstallation is the one decision about which workspace a job runs in.
// River runs it as worker middleware around every job (installationBinding),
// so no worker binds its own.
//
// It looks the workspace up on every call and keeps nothing between jobs: a
// cached answer would outlive a change of the installation's company. It fails
// when no workspace is live, and the job then does not run.
//
// Some stores still read the workspace off the context, as an audit entity id,
// a blob key or an advisory-lock name. Without the binding those would carry a
// zero id with nothing to fail on.
func bindInstallation(ctx context.Context, pool *pgxpool.Pool) (context.Context, error) {
	ws, err := identity.NewService(pool).InstallationWorkspace(ctx)
	if err != nil {
		return nil, err
	}
	return principal.WithWorkspaceID(ctx, ws.UUID), nil
}

// everyWorkspaceKinds are the jobs that clean up data in every workspace row,
// archived ones included, and so must run when no company is live. They run
// through runEveryWorkspacePass, which reads the rows itself, so the binding
// leaves them alone. TestEveryWorkspaceKindsRunTheEveryWorkspacePass holds the
// list to the workers.
var everyWorkspaceKinds = map[string]bool{
	IdempotencyRetentionArgs{}.Kind(): true,
	CaptureTraceSweepArgs{}.Kind():    true,
	StoredObjectReapArgs{}.Kind():     true,
	SuppressionJournalArgs{}.Kind():   true,
}

// searchBoundKinds bind a workspace through the search store. With no live
// workspace, the re-embed hands its marker back and cancels. The middleware
// would fail such a job before it could.
var searchBoundKinds = map[string]bool{
	EmbedReindexArgs{}.Kind(): true,
}

// installationBinding is the jobs.Config.BindWorkspace of the worker client.
func installationBinding(pool *pgxpool.Pool) jobs.BindWorkspace {
	return func(ctx context.Context, job *rivertype.JobRow) (context.Context, error) {
		if everyWorkspaceKinds[job.Kind] || searchBoundKinds[job.Kind] {
			return ctx, nil
		}
		return bindInstallation(ctx, pool)
	}
}

// boundWorkspace reads the workspace installationJobCtx bound, for a worker
// that hands it to a store which still takes one.
func boundWorkspace(ctx context.Context) (ids.UUID, error) {
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return ids.UUID{}, database.ErrNoWorkspace
	}
	return ws, nil
}
