// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// installationJobCtx binds the installation's workspace for a collapsed pass —
// one that carries no workspace of its own (ADR-0103 §1).
//
// The binding is NOT vestigial while any store still reads the workspace off
// the context: storekit.MustWorkspace reads it, sixteen deals sites and a
// handful in contacts, capture, ai and activities call it, and an agent tool
// reaches several of them — as an audit entity id, a blob storage key or an
// advisory-lock name, never as a column any of those stores writes. Unbound,
// those calls would silently carry a zero uuid into whichever of those it
// feeds, with nothing to fail loudly on. It retires with MustWorkspace itself
// (ADR-0091 §5), not before.
func installationJobCtx(ctx context.Context, svc *identity.Service) (context.Context, error) {
	ws, err := svc.InstallationWorkspace(ctx)
	if err != nil {
		return nil, err
	}
	return principal.WithWorkspaceID(ctx, ws.UUID), nil
}

// bindInstallation is installationJobCtx for a worker that holds a pool and no
// identity service. It resolves the installation's workspace per job, so a job
// queued before the first company existed fails until one does.
func bindInstallation(ctx context.Context, pool *pgxpool.Pool) (context.Context, error) {
	return installationJobCtx(ctx, identity.NewService(pool))
}

// installationBinder is bindInstallation as the Bind hook the capture and chat
// workers of the edge packages take.
func installationBinder(pool *pgxpool.Pool) func(context.Context) (context.Context, error) {
	return func(ctx context.Context) (context.Context, error) { return bindInstallation(ctx, pool) }
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
