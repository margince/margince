// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func visibleRelinkPredicate(ctx context.Context, id ids.ActivityID, entityType, column string) (string, []any, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idPos, typePos := arg(id), arg(entityType)
	scope, err := auth.ScopeClauseFor(ctx, entityType, "t", arg)
	if err != nil {
		return "", nil, err
	}
	if scope == "" {
		scope = "true"
	}
	return storekit.SQLf(`activity_id = $%d AND entity_type = $%d
 AND EXISTS (SELECT 1 FROM %s t WHERE t.id = activity_link.%s AND %s)`,
		idPos, typePos, entityType, column, scope), args, nil
}

func visibleRelinkTargets(ctx context.Context, tx pgx.Tx, id ids.ActivityID, entityType, column string) ([]ids.UUID, error) {
	predicate, args, err := visibleRelinkPredicate(ctx, id, entityType, column)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, storekit.SQLf("SELECT %s FROM activity_link WHERE ", column)+predicate+" ORDER BY id", args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}
