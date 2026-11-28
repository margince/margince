// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river/rivertype"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/jobs"
)

// bindInstallation is the one decision whether a job may run: the installation
// has one live company. River runs it as worker middleware around every
// job (installationBinding), so no worker checks for itself.
//
// It looks the company up on every call and keeps nothing between jobs: a
// cached answer would outlive a change of the installation's company. It fails
// when no company is live, and the job then does not run. The context comes back
// unchanged, because a store that names the company asks its own handle.
func bindInstallation(ctx context.Context, pool *pgxpool.Pool) (context.Context, error) {
	if _, err := identity.NewService(pool).InstallationWorkspace(ctx); err != nil {
		return nil, err
	}
	return ctx, nil
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
