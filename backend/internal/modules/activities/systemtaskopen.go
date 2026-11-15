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
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// KeepOneSystemTaskOnTx keeps one open task and refreshes unusable evidence before adoption.
// The caller serialises arrivals on the record, including owner changes.
func KeepOneSystemTaskOnTx(ctx context.Context, tx pgx.Tx, sourceSystem, entityType string, entityID, source ids.UUID, asOf time.Time) (bool, error) {
	column := linkColumn(entityType)
	if column == "" {
		return false, &InvalidLinkTypeError{EntityType: entityType}
	}
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }
	entityArg, sourceArg := arg(entityID), arg(sourceSystem)
	var open ids.UUID
	err := tx.QueryRow(ctx, storekit.SQLf(`SELECT t.id
		  FROM activity t JOIN activity_link l ON l.activity_id = t.id
		 WHERE l.%s = $%d AND t.kind = 'task' AND t.source_system = $%d
		   AND t.is_done = false AND t.archived_at IS NULL
		 ORDER BY t.created_at DESC, t.id DESC LIMIT 1 FOR UPDATE OF t`, column, entityArg, sourceArg),
		args...).Scan(&open)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("activities: reading the open %s task on a %s: %w", sourceSystem, entityType, err)
	}
	if source == ids.Nil {
		return true, nil
	}
	if err := lockTaskAdoption(ctx, tx, open.String()); err != nil {
		return false, err
	}
	args = nil
	openArg := arg(open)
	current := storekit.CurrentTaskClause("t", asOf, "'[]'::jsonb", arg)
	var isCurrent, held bool
	var prior, owner *ids.UUID
	query := storekit.SQLf(`SELECT %s, %s, %s, t.assignee_id
 FROM activity t WHERE t.id=$%d AND t.archived_at IS NULL`, current, storekit.HeldTaskClause("t"), provenance.TaskSourceActivitySQL("t"), openArg)
	if err := tx.QueryRow(ctx, query, args...).Scan(&isCurrent, &held, &prior, &owner); err != nil {
		return false, fmt.Errorf("activities: reading the open task's source and adoption: %w", err)
	}
	if held {
		return true, nil
	}
	usable := false
	if prior != nil {
		usable, err = CurrentLeadEngagementTx(ctx, tx, *prior, owner, asOf)
		if err != nil {
			return false, err
		}
	}
	if !isCurrent || !usable {
		eligible, err := CurrentLeadEngagementTx(ctx, tx, source, owner, asOf)
		if err != nil {
			return false, err
		}
		if !eligible {
			return true, nil
		}
		if _, err := updateActivityInTx(ctx, tx, ids.From[ids.ActivityKind](open), UpdateActivityInput{sourceActivityID: &source, recordedAt: &asOf}); err != nil {
			return false, fmt.Errorf("activities: pointing the open %s task at its new evidence: %w", sourceSystem, err)
		}
	}
	return true, nil
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
