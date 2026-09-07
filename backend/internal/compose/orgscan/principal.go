// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package orgscan

// Who a scan runs as. The worker runs on a job, but everything it reads must
// be exactly what the reader could see themselves — the whole security
// argument of a per-reader scan — so the job re-binds that human's OWN
// authority rather than a system principal with the human's name on it. A
// system principal reads every audience away, and a scan assembled under one
// would quote a message the reader may not open.

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// Authority resolves a human's grants and seat as one snapshot.
type Authority interface {
	EffectiveAuthority(ctx context.Context, workspaceID, humanID ids.UUID) (authz.RBAC, principal.SeatType, error)
}

// WorkerContext binds the viewer's own principal and the scan's correlation
// onto a job context. The correlation id IS the row id, so every trace and
// every audit row for one read is joinable on the scan.
func WorkerContext(ctx context.Context, authority Authority, viewer ids.UserID, scanID ids.UUID) (context.Context, error) {
	workspace, ok := principal.WorkspaceID(ctx)
	if !ok {
		return nil, fmt.Errorf("orgscan: the job context carries no workspace")
	}
	rbac, seat, err := authority.EffectiveAuthority(ctx, workspace, viewer.UUID)
	if err != nil {
		return nil, fmt.Errorf("orgscan: resolving the reader's authority: %w", err)
	}
	ctx = principal.WithActor(ctx, principal.Principal{
		Type:        principal.PrincipalHuman,
		ID:          "human:" + viewer.String(),
		UserID:      viewer.UUID,
		SeatType:    seat,
		TeamIDs:     rbac.TeamIDs,
		Permissions: rbac.Permissions,
	})
	return principal.WithCorrelationID(ctx, scanID), nil
}
