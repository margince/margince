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
// An open task nobody holds goes to the record's owner, when it has one.
// The caller serialises arrivals on the record, including owner changes.
func KeepOneSystemTaskOnTx(ctx context.Context, tx pgx.Tx, sourceSystem, entityType string, entityID, source ids.UUID, recordOwner *ids.UUID, asOf time.Time) (bool, error) {
	open, found, err := lockOpenSystemTask(ctx, tx, sourceSystem, entityType, entityID)
	if err != nil || !found {
		return false, err
	}
	if source == ids.Nil {
		return true, nil
	}
	if err := lockTaskAdoption(ctx, tx, open.String()); err != nil {
		return false, err
	}
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }
	openArg := arg(open)
	current := storekit.CurrentTaskClause("t", asOf, "'[]'::jsonb", arg)
	var isCurrent, held bool
	var prior, owner *ids.UUID
	query := storekit.SQLf(`SELECT %s, %s, %s, t.assignee_id
 FROM activity t WHERE t.id=$%d AND t.archived_at IS NULL`, current, storekit.HeldTaskClause("t"), provenance.TaskSourceActivitySQL("t"), openArg)
	if err := tx.QueryRow(ctx, query, args...).Scan(&isCurrent, &held, &prior, &owner); err != nil {
		return false, fmt.Errorf("activities: reading the open task's source and adoption: %w", err)
	}
	if owner == nil && recordOwner != nil {
		if err := handTaskToOwner(ctx, tx, open, *recordOwner, sourceSystem); err != nil {
			return false, err
		}
		owner = recordOwner
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
		if err := repointTaskEvidence(ctx, tx, open, source, owner, asOf, sourceSystem); err != nil {
			return false, err
		}
	}
	return true, nil
}

// repointTaskEvidence points an open task at the new source when that source
// is usable for the task's assignee, and leaves the task alone otherwise.
func repointTaskEvidence(ctx context.Context, tx pgx.Tx, task, source ids.UUID, owner *ids.UUID, asOf time.Time, sourceSystem string) error {
	eligible, err := CurrentLeadEngagementTx(ctx, tx, source, owner, asOf)
	if err != nil || !eligible {
		return err
	}
	if _, err := updateActivityInTx(ctx, tx, ids.From[ids.ActivityKind](task), UpdateActivityInput{sourceActivityID: &source, recordedAt: &asOf}); err != nil {
		return fmt.Errorf("activities: pointing the open %s task at its new evidence: %w", sourceSystem, err)
	}
	return nil
}

// lockOpenSystemTask finds the newest open task from this source on the
// record and locks its row, so a second arrival waits for the first.
func lockOpenSystemTask(ctx context.Context, tx pgx.Tx, sourceSystem, entityType string, entityID ids.UUID) (ids.UUID, bool, error) {
	column := linkColumn(entityType)
	if column == "" {
		return ids.Nil, false, &InvalidLinkTypeError{EntityType: entityType}
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
		return ids.Nil, false, nil
	}
	if err != nil {
		return ids.Nil, false, fmt.Errorf("activities: reading the open %s task on a %s: %w", sourceSystem, entityType, err)
	}
	return open, true, nil
}

// handTaskToOwner assigns an open task nobody holds to the record's owner.
func handTaskToOwner(ctx context.Context, tx pgx.Tx, task, recordOwner ids.UUID, sourceSystem string) error {
	assignee := ids.From[ids.UserKind](recordOwner)
	if _, err := updateActivityInTx(ctx, tx, ids.From[ids.ActivityKind](task), UpdateActivityInput{AssigneeID: &assignee}); err != nil {
		return fmt.Errorf("activities: handing the open %s task to the record's owner: %w", sourceSystem, err)
	}
	return nil
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

// SystemTaskFiledOnTx reports whether a task from this source and key is filed
// on the record, done or archived included, as LogActivityTx's replay check does.
func SystemTaskFiledOnTx(ctx context.Context, tx pgx.Tx, sourceSystem, sourceID, entityType string, entityID ids.UUID) (bool, error) {
	filed, err := SourceFiledOnTx(ctx, tx, sourceSystem, []string{storekit.EscapeLike(sourceID)}, entityType, entityID)
	return !filed.IsZero(), err
}

// SourceFiledOnTx answers an activity from this source, under a key matching
// one of the LIKE patterns, that is filed on the record. Done and archived
// rows count, as in LogActivityTx's replay check. None answers the zero id.
func SourceFiledOnTx(ctx context.Context, tx pgx.Tx, sourceSystem string, keyPatterns []string, entityType string, entityID ids.UUID) (ids.UUID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return ids.Nil, err
	}
	column := linkColumn(entityType)
	if column == "" {
		return ids.Nil, &InvalidLinkTypeError{EntityType: entityType}
	}
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }
	query := storekit.SQLf(`SELECT t.id FROM activity t JOIN activity_link l ON l.activity_id = t.id
		 WHERE t.source_system = $%d AND t.source_id LIKE ANY($%d) AND l.%s = $%d
		 ORDER BY t.id LIMIT 1`, arg(sourceSystem), arg(keyPatterns), column, arg(entityID))
	var filed ids.UUID
	err := tx.QueryRow(ctx, query, args...).Scan(&filed)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, nil
	}
	if err != nil {
		return ids.Nil, fmt.Errorf("activities: reading the %s activity on a %s: %w", sourceSystem, entityType, err)
	}
	return filed, nil
}
