// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// markAssignable answers both owner checks: a record's owner needs the seat
// alone, and an automation's owner must also lie inside the caller's scope.
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
