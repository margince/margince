// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// CurrentTaskClause admits current automatic promises, addressed requests and human work
// in record task reads; alias names a compile-time activity alias, never request input.
func CurrentTaskClause(alias string, asOf time.Time, identities string, arg func(any) int) string {
	return storekit.CurrentTaskClause(alias, asOf, identities, arg)
}

func currentTaskMailIdentities(ctx context.Context, tx pgx.Tx, in ListActivitiesInput) (map[string][]string, error) {
	if in.WorklistAsOf == nil && in.AssigneeID == nil && in.RequestCurrentAsOf == nil {
		return map[string][]string{}, nil
	}
	return storekit.UnambiguousMailSeatIdentities(ctx, tx)
}
