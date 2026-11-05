// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// markAssignable answers, per row, the owner-change check the write would run.
// That check is the seat alone, so the two roster fields carry one answer.
func (s *Service) markAssignable(ctx context.Context, rows []userRow) error {
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		eligible, err := auth.EligibleAssignees(ctx, tx, rowIDs(rows))
		if err != nil {
			return err
		}
		for i := range rows {
			assignable := eligible[rows[i].ID]
			rows[i].Assignable, rows[i].CanOwnRecords = &assignable, &assignable
		}
		return nil
	})
}
