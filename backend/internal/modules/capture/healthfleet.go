// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// FleetConcern associates a health concern with its owner for authorized operators.
type FleetConcern struct {
	UserID      ids.UUID
	DisplayName *string
	Connection  ConnectionView
}

// FleetConcernsTx exposes connection state to human health readers, never correspondence.
func FleetConcernsTx(ctx context.Context, tx pgx.Tx, now time.Time) ([]FleetConcern, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return nil, apperrors.ErrPermissionDenied
	}
	if err := auth.Require(ctx, "job_health", principal.ActionRead); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT c.user_id, u.display_name, c.id, c.provider, c.status,
  c.account_label, c.connected_at, c.silence_dismissed_since,
  s.last_delivery_at, s.last_success_at, s.last_error_class, s.failing_since,
  b.status, b.completed_at
 FROM capture_connection c JOIN app_user u ON u.id = c.user_id AND `+workforce.LiveMemberSQL("u")+`
 LEFT JOIN capture_sync_state s ON s.connection_id = c.id
 LEFT JOIN LATERAL (SELECT status, completed_at FROM capture_backfill WHERE connection_id = c.id ORDER BY created_at DESC, id DESC LIMIT 1) b ON true
 WHERE c.archived_at IS NULL AND c.status <> 'disconnected'`)
	if err != nil {
		return nil, fmt.Errorf("capture: reading connection health: %w", err)
	}
	defer rows.Close()
	out := []FleetConcern{}
	for rows.Next() {
		var row FleetConcern
		var status *string
		var completed *time.Time
		v := &row.Connection
		if err := rows.Scan(&row.UserID, &row.DisplayName, &v.ID, &v.Provider, &v.Status,
			&v.AccountLabel, &v.ConnectedAt, &v.SilenceDismissedSince, &v.LastDeliveryAt, &v.LastSuccessAt,
			&v.LastErrorClass, &v.FailingSince, &status, &completed); err != nil {
			return nil, err
		}
		if status != nil {
			v.Backfill = &BackfillRun{Status: *status, CompletedAt: completed}
		}
		classifyConnection(v, now)
		if v.Concern != "" {
			out = append(out, row)
		}
	}
	slices.SortFunc(out, func(a, b FleetConcern) int {
		if d := slices.Index(ConcernKinds, a.Connection.Concern) - slices.Index(ConcernKinds, b.Connection.Concern); d != 0 {
			return d
		}
		if a.Connection.ConcernSince != nil && b.Connection.ConcernSince != nil {
			if order := a.Connection.ConcernSince.Compare(*b.Connection.ConcernSince); order != 0 {
				return order
			}
		}
		return strings.Compare(a.Connection.ID.String(), b.Connection.ID.String())
	})
	return out, rows.Err()
}
