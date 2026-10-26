// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// connectorActing is ctx acting as the named connector for grantedBy, with
// that human's live authority. Capture syncs and the deal repair both build
// it here, so a repair decides under the scope the capture ran with.
func connectorActing(ctx context.Context, authority authz.Resolver, name string, grantedBy ids.UserID) (context.Context, error) {
	wsID, ok := principal.WorkspaceID(ctx)
	if !ok {
		return nil, errors.New("capture: sync outside workspace context")
	}
	// The authz resolver and the principal seam are untyped (ids.UUID);
	// widen the typed granting-human id at each of those edges.
	rbac, err := authority.EffectiveRBAC(ctx, wsID, grantedBy.UUID)
	if err != nil {
		return nil, fmt.Errorf("capture: granting human no longer resolves — the grant dies with them: %w", err)
	}
	seat, err := authority.SeatType(ctx, wsID, grantedBy.UUID)
	if err != nil {
		return nil, err
	}
	p := principal.Principal{
		Type:        principal.PrincipalConnector,
		ID:          connectorPrincipalID(name),
		UserID:      grantedBy.UUID,
		OnBehalfOf:  grantedBy.UUID,
		TeamIDs:     rbac.TeamIDs,
		SeatType:    seat,
		Permissions: rbac.Permissions,
	}
	return principal.WithActor(ctx, p), nil
}
