// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package overlay

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// boundWorkspace names the workspace this service's transactions run against
// — the row a statement that writes `workspace` itself must target.
//
// It asks the DATABASE HANDLE rather than the request context because the
// handle is what set the binding: DB.Tx resolves the installation's workspace
// through its own resolver and never consults ctx. Reading ctx here would make
// two sources for one fact, and they answer differently on a handle pinned to
// a workspace the caller's context does not name (database.BindTo — the flip
// lane's acting-workspace handle, and the cross-tenant suites). The row the
// transaction is bound to is the only correct answer, so it is the one asked
// for.
func (s *Service) boundWorkspace(ctx context.Context) (ids.UUID, error) {
	ws, err := s.db.Workspace(ctx)
	if err != nil {
		return ids.UUID{}, fmt.Errorf("overlay: resolving the workspace this transaction binds: %w", err)
	}
	return ws.UUID, nil
}
