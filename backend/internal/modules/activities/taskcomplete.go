// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CompleteTask completes one task because the thing it tracked was settled
// somewhere else, and answers whether this call is what completed it. A task
// already done, or gone, is not an error: the settlement it follows has
// nothing left to do. It completes through UpdateActivity, so the task's own
// gates, audit and event apply as they do to a human ticking it.
func (s *Store) CompleteTask(ctx context.Context, id ids.ActivityID) (bool, error) {
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
	if (current.IsDone != nil && *current.IsDone) || current.Version == nil {
		return false, nil
	}
	return s.completeSystemTask(ctx, id, int64(*current.Version))
}
