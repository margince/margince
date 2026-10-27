// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// OpenSystemTaskOnTx reports whether the system already has an open task from
// this source on the record. The caller serialises it with its own write.
func OpenSystemTaskOnTx(ctx context.Context, tx pgx.Tx, sourceSystem, entityType string, entityID ids.UUID, asOf time.Time, owner *ids.UUID) (bool, error) {
	column := linkColumn(entityType)
	if column == "" {
		return false, &InvalidLinkTypeError{EntityType: entityType}
	}
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }
	entityArg, sourceArg := arg(entityID), arg(sourceSystem)
	current := storekit.CurrentTaskClause("t", asOf, "'[]'::jsonb", arg)
	if owner != nil {
		seat, err := auth.SeatPrincipal(ctx, tx, *owner)
		if err != nil {
			return false, err
		}
		visible, err := auth.ActivityContentClause(principal.WithActor(ctx, seat), "t", arg)
		if err != nil {
			return false, err
		}
		current += " AND " + visible
	}
	var open bool
	err := tx.QueryRow(ctx, storekit.SQLf(`SELECT EXISTS (
		SELECT 1 FROM activity t JOIN activity_link l ON l.activity_id = t.id
		 WHERE l.%s = $%d AND t.kind = 'task' AND t.source_system = $%d
		   AND t.is_done = false AND t.archived_at IS NULL AND %s)`, column, entityArg, sourceArg, current),
		args...).Scan(&open)
	if err != nil {
		return false, fmt.Errorf("activities: reading the open %s task on a %s: %w", sourceSystem, entityType, err)
	}
	return open, nil
}

// CurrentLeadEngagementTx applies source admission and the prospective assignee's access.
func CurrentLeadEngagementTx(ctx context.Context, tx pgx.Tx, source ids.UUID, owner *ids.UUID, asOf time.Time) (bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return false, err
	}
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }
	sourceRef := fmt.Sprintf("$%d::uuid", arg(source))
	clause := storekit.CurrentLeadEngagementClause(sourceRef, asOf, arg)
	var current bool
	if err := tx.QueryRow(ctx, "SELECT "+clause, args...).Scan(&current); err != nil {
		return false, fmt.Errorf("activities: reading lead engagement: %w", err)
	}
	if !current {
		return false, nil
	}
	if owner != nil {
		seat, err := auth.SeatPrincipal(ctx, tx, *owner)
		if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("activities: reading lead owner authority: %w", err)
		}
		err = auth.EnsureActivityContentVisibleLive(principal.WithActor(ctx, seat), tx, source)
		if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
	return true, nil
}
