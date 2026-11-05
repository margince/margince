// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CompleteTask preserves changes made after the settlement that caused completion.
func (s *Store) CompleteTask(ctx context.Context, id ids.ActivityID, settledAt time.Time) (bool, error) {
	var completed bool
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		completed, err = s.CompleteTaskTx(ctx, tx, id, settledAt)
		return err
	})
	return completed, err
}

// CompleteTaskTx holds the task lock so callers can check related obligations in the same transaction.
func (s *Store) CompleteTaskTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, settledAt time.Time) (bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return false, err
	}
	if err := LockActivityLive(ctx, tx, id); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	current, err := readActivity(ctx, tx, id, storekit.LiveOnly)
	if errors.Is(err, apperrors.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if current.Kind != crmcontracts.ActivityKindTask {
		return false, fmt.Errorf("activities: %s is a %s, not a task to complete: %w", id, current.Kind, apperrors.ErrConflict)
	}
	if (current.IsDone != nil && *current.IsDone) || current.Version == nil || current.UpdatedAt.After(settledAt) {
		return false, nil
	}
	_, err = s.SetTaskDoneTx(ctx, tx, id, true, int64(*current.Version))
	return err == nil, err
}
