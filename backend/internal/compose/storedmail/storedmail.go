// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package storedmail pages captured mail whose stored original survives, for
// the data tasks that correct a row from its original.
package storedmail

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/originalstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Page reads one page of stored mail and applies fix to every row it
// reads, locking each against a concurrent write.
//
// One function for every step that corrects stored mail from its original.
// Each supplies the columns it needs and the scope that selects them.
//
// Each also advances over rows that needed nothing, because whether a row needs
// its step is decided per row, against the re-parse.
func Page[T any](
	ctx context.Context, tx pgx.Tx, in datatask.BatchIn, columns, scope string,
	originalOf func(*T) (ids.UUID, *[]byte),
	fix func(context.Context, pgx.Tx, T) (bool, error),
) (datatask.BatchOut, error) {
	return readPage(ctx, tx, in, columns, scope, " FOR UPDATE OF a", originalOf, fix)
}

// AttendeePage is Page for a step whose fix writes the meetings' attendee rows.
// It takes their attendee locks before their row locks, as a judge needs.
func AttendeePage[T any](
	ctx context.Context, tx pgx.Tx, in datatask.BatchIn, columns, scope string,
	originalOf func(*T) (ids.UUID, *[]byte),
	fix func(context.Context, pgx.Tx, T) (bool, error),
) (datatask.BatchOut, error) {
	return readPage(ctx, tx, in, columns, scope, "", originalOf, fix)
}

// readPage reads with lock as its row lock. Without one it takes
// storekit.LockMeetingAttendeePage over the rows it read.
func readPage[T any](
	ctx context.Context, tx pgx.Tx, in datatask.BatchIn, columns, scope, lock string,
	originalOf func(*T) (ids.UUID, *[]byte),
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
		" AND a.id > @stored_mail_after ORDER BY a.id LIMIT @stored_mail_limit"+lock,
		pgx.NamedArgs{"stored_mail_after": after, "stored_mail_limit": in.Limit})
	if err != nil {
		return datatask.BatchOut{}, err
	}
	candidates, err := pgx.CollectRows(rows, pgx.RowToStructByPos[T])
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned := make([]ids.UUID, 0, len(candidates))
	for i := range candidates {
		id, _ := originalOf(&candidates[i])
		scanned = append(scanned, id)
	}
	if lock == "" {
		if err := storekit.LockMeetingAttendeePage(ctx, tx, scanned); err != nil {
			return datatask.BatchOut{}, err
		}
	}
	changed := 0
	for _, candidate := range candidates {
		_, payload := originalOf(&candidate)
		*payload, err = originalstore.RepairPayload(ctx, *payload)
		if err != nil {
			return datatask.BatchOut{}, err
		}
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

// Count counts the rows a scope holds, up to one past the cap it is
// given, so a caller can say whether more are waiting.
func Count(ctx context.Context, q datatask.Querier, scope string, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, "SELECT count(*) FROM (SELECT a.id "+scope+" LIMIT @stored_mail_cap) candidates",
		pgx.NamedArgs{"stored_mail_cap": maximum + 1}).Scan(&count)
	return min(count, maximum), count > maximum, err
}
