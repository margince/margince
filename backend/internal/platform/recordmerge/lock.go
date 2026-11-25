// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package recordmerge

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Screen answers the refusal Pair would give, before any row lock is taken. That
// is a source the caller cannot write or a target the caller cannot change.
// A target that does not exist answers the same conflict as one out of scope.
// The merge then cannot probe which ids exist. Pair asks again under the lock.
func Screen(ctx context.Context, tx pgx.Tx, kind string, sourceID, targetID ids.UUID) error {
	if err := auth.EnsureWritable(ctx, tx, kind, sourceID); err != nil {
		return err
	}
	writable, err := auth.WritableBy(ctx, tx, kind, targetID)
	if err != nil {
		return err
	}
	if !writable {
		return apperrors.ErrConflict
	}
	return nil
}

// MissingIsConflict maps the not-found storekit.LockPair answers for a row that
// vanished since Screen to the conflict an out-of-scope target answers.
func MissingIsConflict(err error) error {
	if errors.Is(err, apperrors.ErrNotFound) {
		return apperrors.ErrConflict
	}
	return err
}
