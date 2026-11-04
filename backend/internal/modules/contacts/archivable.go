// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// stampArchivable sets each row's archivable flag from auth.ArchivableSubset,
// which asks the row write gate and the archive rule together, so the flag
// answers what the archive itself would.
func stampArchivable[T any](
	ctx context.Context, tx pgx.Tx, rule auth.ArchiveRule, rows []T, id func(T) ids.UUID, set func(*T, bool),
) error {
	named := make([]ids.UUID, len(rows))
	for i := range rows {
		named[i] = id(rows[i])
	}
	archivable, err := auth.ArchivableSubset(ctx, tx, rule, named)
	if err != nil {
		return err
	}
	for i := range rows {
		set(&rows[i], archivable[named[i]])
	}
	return nil
}
