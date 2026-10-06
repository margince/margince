// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// A bulk change marking Worklist tasks done reaches each task here: locked and
// read as the change found it, then ticked through updateActivityInTx, the
// write a PATCH of is_done makes, so each task gets its own audit row and event.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// BulkTask is one task as a bulk change found it under its row lock.
type BulkTask struct {
	Subject string
	Version int64
	Done    bool
}

// LockTaskForBulkTx takes the row lock a bulk change holds for its whole
// transaction. A row that is not a live task whose content the caller may
// read answers apperrors.ErrNotFound.
func (s *Store) LockTaskForBulkTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (BulkTask, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return BulkTask{}, err
	}
	if err := auth.EnsureActivityContentVisible(ctx, tx, id.UUID); err != nil {
		return BulkTask{}, err
	}
	var task BulkTask
	err := tx.QueryRow(ctx, `
		SELECT coalesce(subject, ''), version, is_done FROM activity
		 WHERE id = $1 AND kind = 'task' AND archived_at IS NULL
		   FOR UPDATE`, id.UUID).Scan(&task.Subject, &task.Version, &task.Done)
	if errors.Is(err, pgx.ErrNoRows) {
		return BulkTask{}, apperrors.ErrNotFound
	}
	if err != nil {
		return BulkTask{}, fmt.Errorf("lock task %s for a bulk change: %w", id, err)
	}
	return task, nil
}

// SetTaskDoneTx marks one task done or open again, conditioned on ifVersion,
// and answers the version the write left it at.
func (s *Store) SetTaskDoneTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, done bool, ifVersion int64) (int64, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return 0, err
	}
	at := s.now()
	out, err := updateActivityInTx(ctx, tx, id, UpdateActivityInput{recordedAt: &at, IsDone: &done, IfVersion: &ifVersion})
	if err != nil {
		return 0, err
	}
	if out.Version == nil {
		return 0, fmt.Errorf("activities: task %s read back without a version", id)
	}
	return *out.Version, nil
}

// WritableTasks answers which of the named tasks the caller could mark done, so
// a list offers Done only where the write would be admitted.
func (s *Store) WritableTasks(ctx context.Context, named []ids.UUID) (map[ids.UUID]bool, error) {
	err := auth.Require(ctx, "activity", principal.ActionUpdate)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return map[ids.UUID]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	var out map[ids.UUID]bool
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = auth.ActivityWritableSubset(ctx, tx, named)
		return err
	})
	return out, err
}
