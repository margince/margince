// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package personalchannelwire

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/personalchannels"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// NewHandlers serves /personal-channels with pushed messages landing through sink.
func NewHandlers(pool *pgxpool.Pool, db *database.DB, sink *capture.Sink) personalchannels.Handlers {
	return personalchannels.NewHandlers(NewStore(pool, db, sink))
}

// NewStore is the module's store with pushed messages landing through sink.
func NewStore(pool *pgxpool.Pool, db *database.DB, sink *capture.Sink) *personalchannels.Store {
	return personalchannels.NewStore(db).WithSettler(Settler{}).
		WithLander(Lander{Sink: sink, DB: db, Authority: liveAuthority(pool)})
}

// liveAuthority binds the capture connector for member with the grants member
// holds now, as a unit's ingress does. A pushed message is the member's own
// correspondence, so it lands under their authority and never the client's.
func liveAuthority(pool *pgxpool.Pool) Authority {
	return func(ctx context.Context, member ids.UUID) (context.Context, error) {
		ws, ok := principal.WorkspaceID(ctx)
		if !ok {
			return nil, fmt.Errorf("personal channels: no workspace bound: %w", apperrors.ErrPermissionDenied)
		}
		rbac, seat, err := identity.NewService(pool).EffectiveAuthority(ctx, ws, member)
		if err != nil {
			return nil, err
		}
		acting := principal.Principal{
			Type: principal.PrincipalConnector, ID: ConnectorID, UserID: member, OnBehalfOf: member,
			TeamIDs: rbac.TeamIDs, SeatType: seat, Permissions: rbac.Permissions,
		}
		return principal.WithCorrelationID(principal.WithActor(ctx, acting), ids.NewV7()), nil
	}
}
