// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package recordmerge

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Pair resolves and validates both ends. A merge changes both records, so both
// carry the write-authority probe and not the visibility one. A colleague with
// a `read` share of either record may not spend it on a merge.
//
// The source must be live and writable. A source already merged away answers
// 409 with the pointer, which discloses nothing new because the caller just
// proved they can address the row.
//
// The target must be live and writable too (ensureTarget).
func Pair[T any, K ids.EntityKind](ctx context.Context, tx pgx.Tx, kind string, sourceID, targetID ids.ID[K],
	ifVersion *int64,
	read func(context.Context, pgx.Tx, ids.ID[K]) (T, *ids.UUID, error),
) (source, target T, err error) {
	var zero T
	if err := auth.EnsureWritable(ctx, tx, kind, sourceID.UUID); err != nil {
		return zero, zero, err
	}
	source, mergedInto, err := read(ctx, tx, sourceID)
	if err != nil {
		if mergedInto != nil && !mergedInto.IsZero() {
			return zero, zero, &AlreadyMergedError{Kind: kind, IntoID: *mergedInto}
		}
		return zero, zero, err
	}

	if err := requireSourceVersion(ctx, tx, kind, sourceID.UUID, ifVersion); err != nil {
		return zero, zero, err
	}

	if err := ensureTarget(ctx, tx, kind, targetID.UUID); err != nil {
		return zero, zero, err
	}
	target, _, err = read(ctx, tx, targetID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return zero, zero, &MergedTargetError{Kind: kind}
		}
		return zero, zero, err
	}
	for _, id := range []ids.UUID{sourceID.UUID, targetID.UUID} {
		if err := storekit.EnsureReleasedVersion(ctx, tx, kind, id); err != nil {
			return zero, zero, err
		}
	}
	return source, target, nil
}

// ensureTarget refuses a target the caller cannot change. One they cannot see,
// or one no row holds, answers the bare conflict. Merging returns the survivor,
// so the refusal must not confirm that it exists. One they may read answers 403
// with the reason, which a retry with a fresh version can never cure.
func ensureTarget(ctx context.Context, tx pgx.Tx, kind string, targetID ids.UUID) error {
	writable, err := auth.WritableBy(ctx, tx, kind, targetID)
	if err != nil || writable {
		return err
	}
	visible, err := auth.VisibleToExisting(ctx, tx, kind, targetID)
	switch {
	case err != nil:
		return err
	case visible:
		return errTargetReadOnly
	}
	return apperrors.ErrConflict
}

// requireSourceVersion refuses the merge when the source's version differs from
// the caller's If-Match. The pair lock is already held, so the version checked
// is the one merged. Nil means no precondition.
func requireSourceVersion(ctx context.Context, tx pgx.Tx, kind string, sourceID ids.UUID, ifVersion *int64) error {
	if ifVersion == nil {
		return nil
	}
	var current int64
	err := tx.QueryRow(ctx, "SELECT version FROM "+pgx.Identifier{kind}.Sanitize()+" WHERE id = $1", sourceID).Scan(&current)
	if err != nil {
		return err
	}
	if current != *ifVersion {
		return apperrors.ErrVersionSkew
	}
	return nil
}

// ArchiveMergedAway retires the source row with its redirect pointer in one
// statement, so a concurrent merge of the same source loses the race.
func ArchiveMergedAway(ctx context.Context, tx pgx.Tx, table string, sourceID, targetID ids.UUID) error {
	tag, err := tx.Exec(ctx,
		`UPDATE `+table+` SET archived_at = $3, merged_into_id = $2
		 WHERE id = $1 AND archived_at IS NULL`,
		sourceID, targetID, time.Now().UTC())
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperrors.ErrConflict
	}
	return nil
}
