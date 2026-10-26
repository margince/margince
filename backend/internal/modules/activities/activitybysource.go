// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ActivityIDBySource finds the activity another system calls (system, sourceID).
// An activity the caller may not read answers ErrNotFound, like one that does
// not exist. The lookup cannot be used to learn which keys are taken.
func (s *Store) ActivityIDBySource(ctx context.Context, system, sourceID string) (ids.ActivityID, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return ids.ActivityID{}, err
	}
	var found ids.ActivityID
	err := s.tx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`SELECT id FROM activity WHERE source_system = $1 AND source_id = $2`, system, sourceID).Scan(&found)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		_, err = readActivity(ctx, tx, found, storekit.LiveOnly)
		return err
	})
	return found, err
}
