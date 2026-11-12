// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package initiativeenrolment runs the writes that date sales initiative
// membership outside a request: the periodic sweep and the upgrade task. The
// writer itself is initiatives.Store; compose wires both into the job runner.
package initiativeenrolment

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/initiatives"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Sweep dates the membership of every live initiative. Compose walks the
// workspaces on Pool and hands each one to Workspace.
type Sweep struct {
	Store *initiatives.Store
	Pool  *pgxpool.Pool
}

// Workspace dates the live initiatives of one workspace, as the system.
func (s Sweep) Workspace(ctx context.Context, workspace ids.UUID) error {
	ctx = principal.SystemActing(principal.WithWorkspaceID(ctx, workspace), "system:initiative-enrolment")
	_, err := s.Store.SyncLiveEnrolments(ctx)
	return err
}
