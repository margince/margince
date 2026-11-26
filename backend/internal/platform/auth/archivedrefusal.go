// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
)

// RowArchived reports whether one row of table is archived. A row that does not
// exist is apperrors.ErrNotFound. table is a compile-time literal of the caller.
//
// It asks no row scope. The caller has already admitted a reader of the row,
// archived rows included. So no caller who could not open the row learns that
// it is archived.
func RowArchived(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) (bool, error) {
	var archived bool
	err := tx.QueryRow(ctx,
		fmt.Sprintf(`SELECT archived_at IS NOT NULL FROM %s WHERE id = $1`, pgx.Identifier{table}.Sanitize()),
		id).Scan(&archived)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, apperrors.ErrNotFound
	}
	return archived, err
}

// RefuseArchived is the one answer a change to an archived record gets:
// apperrors.ErrArchived (409). Every update path asks it before its write; an
// archive path asks ArchivedAlready instead, because a second archive is a
// no-op and not a refusal.
//
// A row-scoped table is admitted by its row scope first, so a row the caller
// cannot see stays ErrNotFound. A table scoped through another record asks its
// own gate before this.
func RefuseArchived(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) error {
	if ownerScopedTables[table] {
		if err := probeVisible(ctx, tx, table, id, probeExisting); err != nil {
			return err
		}
	}
	archived, err := RowArchived(ctx, tx, table, id)
	if err != nil {
		return err
	}
	if archived {
		return apperrors.ErrArchived
	}
	return nil
}

// ArchivedAlready reports that a row the caller may archive is archived
// already, so a second archive of it is a no-op that writes nothing. It asks
// before the caller's If-Match check, because a retry carries the version the
// first archive moved past.
//
// A row-scoped table is admitted by its write gate first, so a row the caller
// cannot see stays ErrNotFound. A table scoped through another record asks its
// own gate before this.
func ArchivedAlready(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) (bool, error) {
	if ownerScopedTables[table] {
		if err := EnsureWritable(ctx, tx, table, id); err != nil {
			return false, err
		}
	}
	return RowArchived(ctx, tx, table, id)
}

// archivedOrHidden turns a live probe's ErrNotFound into ErrArchived when the
// caller can see the row and the row is archived. Any other refusal, and a row
// the caller cannot see, keeps the probe's own answer.
func archivedOrHidden(ctx context.Context, tx pgx.Tx, table string, id ids.UUID, refusal error) error {
	if !errors.Is(refusal, apperrors.ErrNotFound) || recordkinds.IsExtensionName(table) {
		return refusal
	}
	if err := RefuseArchived(ctx, tx, table, id); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return refusal
		}
		return err
	}
	return refusal
}
