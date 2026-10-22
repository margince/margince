// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// VisibleOwnedIDs lists the live rows of table that owner owns and the caller
// can see. Id order makes two handovers over one owner lock rows alike. It asks
// the record type's read grant and ScopeClauseFor, as the record list does. A
// row the caller could not find in a list is not counted.
func VisibleOwnedIDs(ctx context.Context, tx pgx.Tx, table string, owner ids.UUID) ([]ids.UUID, error) {
	if err := Require(ctx, table, principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	ownerPos := arg(owner)
	visible, err := ScopeClauseFor(ctx, table, "", arg)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`SELECT id FROM %s WHERE owner_id = $%d AND archived_at IS NULL`, table, ownerPos)
	if visible != "" {
		query += " AND " + visible
	}
	rows, err := tx.Query(ctx, query+" ORDER BY id", args...)
	if err != nil {
		return nil, fmt.Errorf("list %s rows owned by %s: %w", table, owner, err)
	}
	owned, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("list %s rows owned by %s: %w", table, owner, err)
	}
	return owned, nil
}
