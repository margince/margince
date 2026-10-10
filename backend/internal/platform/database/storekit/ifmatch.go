// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// EnsureIfMatch answers the caller's If-Match for an update that writes
// nothing. ApplyGuarded compares the version inside its UPDATE. An update that
// returns early on an empty patch never reaches that compare. Without this, a
// stale version would answer 200 with a row the caller did not see. A stale
// version is ErrVersionSkew and a gone row is ErrNotFound, as in ApplyGuarded.
// No If-Match means the caller asked for no check.
func EnsureIfMatch(ctx context.Context, tx pgx.Tx, table string, id ids.UUID, ifVersion *int64) error {
	if ifVersion == nil {
		return nil
	}
	var version int64
	err := tx.QueryRow(ctx, SQLf(`SELECT version FROM %s WHERE id = $1%s`, table, liveClause(LiveOnly)), id).
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
