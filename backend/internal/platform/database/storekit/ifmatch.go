// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func EnsureReleasedVersion(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) error {
	version, pinned := auth.ReleasedRecordVersion(ctx, table, id)
	if !pinned {
		return nil
	}
	return EnsureIfMatch(ctx, tx, table, id, &version)
}

// EnsureIfMatch answers the caller's If-Match for an update that writes
// nothing. ApplyGuarded compares the version inside its UPDATE. An update that
// returns early on an empty patch never reaches that compare. Without this, a
// stale version would answer 200 with a row the caller did not see. A stale
// version is ErrVersionSkew and a gone row is ErrNotFound, as in ApplyGuarded.
// No If-Match means the caller asked for no check.
func EnsureIfMatch(ctx context.Context, tx pgx.Tx, table string, id ids.UUID, ifVersion *int64) error {
	return EnsureIfMatchIn(ctx, tx, table, id, ifVersion, LiveOnly)
}

// EnsureIfMatchIn is EnsureIfMatch against a chosen archive filter, for a
// repeat archive that answers with the row it already archived.
func EnsureIfMatchIn(
	ctx context.Context, tx pgx.Tx, table string, id ids.UUID, ifVersion *int64, archived ArchivedFilter,
) error {
	if ifVersion == nil {
		return nil
	}
	var version int64
	err := tx.QueryRow(ctx, SQLf(`SELECT version FROM %s WHERE id = $1%s`, table, liveClause(archived)), id).
		Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return err
	}
	if version != *ifVersion {
		return apperrors.ErrVersionSkew
	}
	return nil
}
