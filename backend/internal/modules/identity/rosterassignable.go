// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// markAssignable answers, per row, the two owner checks a write would run for
// this caller. A record's owner needs the seat alone. An automation's owner
// also needs to lie inside the caller's own write scope.
func (s *Service) markAssignable(ctx context.Context, rows []userRow) error {
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		seated, err := auth.EligibleAssignees(ctx, tx, rowIDs(rows))
		if err != nil {
			return err
		}
		inScope, err := auth.EligibleRunAsOwners(ctx, tx, rowIDs(rows))
		if err != nil {
			return err
		}
		for i := range rows {
			runAs, canOwn := inScope[rows[i].ID], seated[rows[i].ID]
			rows[i].Assignable, rows[i].CanOwnRecords = &runAs, &canOwn
		}
		return nil
	})
}
