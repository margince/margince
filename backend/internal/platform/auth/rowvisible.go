// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// EnsureVisible applies the row scope to a single-row operation: get,
// update, archive, advance. Out of scope reads as ErrNotFound, so the
// caller cannot tell "not yours" from "not there".
// Activities scope through their links (the activities module's
// link-walk clause); pipelines have no owner and are governed by object
// grants only.
func EnsureVisible(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) error {
	return probeVisible(ctx, tx, table, id, false)
}

// EnsureVisibleExisting is EnsureVisible for a read that serves a record's
// sub-resource: the row must exist for every caller, an unbounded one too.
// With no predicate to apply, EnsureVisible admits an id no row holds.
// A list behind that gate then answers an empty 200 for a record that is not there.
// EnsureVisible stays as it is for work about a row that was removed.
func EnsureVisibleExisting(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) error {
	return probeVisible(ctx, tx, table, id, true)
}

func probeVisible(ctx context.Context, tx pgx.Tx, table string, id ids.UUID, mustExist bool) error {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idPos := arg(id)

	clause, err := ScopeClauseFor(ctx, table, "", arg)
	if err != nil {
		return err
	}
	if clause == "" && !mustExist {
		return nil
	}
	where := fmt.Sprintf("id = $%d", idPos)
	if clause != "" {
		where += " AND " + clause
	}

	var visible bool
	err = tx.QueryRow(ctx,
		fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM %s WHERE %s)`, table, where),
		args...).Scan(&visible)
	if err != nil {
		return err
	}
	if !visible {
		return apperrors.ErrNotFound
	}
	return nil
}
