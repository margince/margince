// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// pageStoredMail reads one page of stored mail and applies fix to every row it
// reads, locking each against a concurrent write.
//
// One function for every step that corrects stored mail from its original.
// Each supplies the columns it needs and the scope that selects them.
//
// Each also advances over rows that needed nothing, because whether a row needs
// its step is decided per row, against the re-parse.
func pageStoredMail[T any](
	ctx context.Context, tx pgx.Tx, in datatask.BatchIn, columns, scope string,
	idOf func(T) ids.UUID,
	fix func(context.Context, pgx.Tx, T) (bool, error),
) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, "SELECT "+columns+" "+scope+
		" AND a.id > @stored_mail_after ORDER BY a.id LIMIT @stored_mail_limit FOR UPDATE OF a",
		pgx.NamedArgs{"stored_mail_after": after, "stored_mail_limit": in.Limit})
	if err != nil {
		return datatask.BatchOut{}, err
	}
	candidates, err := pgx.CollectRows(rows, pgx.RowToStructByPos[T])
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned := make([]ids.UUID, 0, len(candidates))
	changed := 0
	for _, candidate := range candidates {
		scanned = append(scanned, idOf(candidate))
		corrected, err := fix(ctx, tx, candidate)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if corrected {
			changed++
		}
	}
	return datatask.AdvanceUUID(in, scanned, changed)
}

// countStoredMail counts the rows a scope holds, up to one past the cap it is
// given, so a caller can say whether more are waiting.
func countStoredMail(ctx context.Context, q datatask.Querier, scope string, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, "SELECT count(*) FROM (SELECT a.id "+scope+" LIMIT @stored_mail_cap) candidates",
		pgx.NamedArgs{"stored_mail_cap": maximum + 1}).Scan(&count)
	return min(count, maximum), count > maximum, err
}
