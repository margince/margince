// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CompleteTask completes one task because the thing it tracked was settled
// somewhere else at settledAt, and answers whether this call is what completed
// it. A task already done, or gone, is not an error: the settlement it follows
// has nothing left to do. Nor is a task changed after settledAt — reopened,
// most likely — which a settlement older than that change does not overrule.
// settledAt is read off the database's clock, as the task's own updated_at
// is, so the two compare on one clock. It completes through UpdateActivity,
// so the task's own gates, audit and event apply as they do to a human
// ticking it.
func (s *Store) CompleteTask(ctx context.Context, id ids.ActivityID, settledAt time.Time) (bool, error) {
	// Every attempt re-reads and re-asks: a task reopened between a skewed
	// attempt and the next is a change after settledAt like any other.
	for range completionAttempts {
		current, err := s.GetActivity(ctx, id, storekit.LiveOnly)
		if errors.Is(err, apperrors.ErrNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if current.Kind != crmcontracts.ActivityKindTask {
			return false, fmt.Errorf("activities: %s is a %s, not a task to complete: %w",
				id, current.Kind, apperrors.ErrConflict)
		}
		if (current.IsDone != nil && *current.IsDone) || current.Version == nil || current.UpdatedAt.After(settledAt) {
			return false, nil
		}
		done, version := true, int64(*current.Version)
		_, err = s.UpdateActivity(ctx, id, UpdateActivityInput{IsDone: &done, IfVersion: &version})
		if err == nil {
			return true, nil
		}
		if errors.Is(err, apperrors.ErrNotFound) {
			return false, nil
		}
		if !errors.Is(err, apperrors.ErrVersionSkew) {
			return false, err
		}
	}
	return false, fmt.Errorf("activities: task %s kept changing while being completed: %w", id, apperrors.ErrVersionSkew)
}
