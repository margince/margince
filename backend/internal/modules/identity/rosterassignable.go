// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// markAssignable answers, per row, the owner-change check and the offer's
// receiver check for this caller. Each picker then offers whom its write accepts.
func (s *Service) markAssignable(ctx context.Context, rows []userRow) error {
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		eligible, err := auth.EligibleAssignees(ctx, tx, rowIDs(rows))
		if err != nil {
			return err
		}
		receivers, err := auth.EligibleHandOverRecipients(ctx, tx, rowIDs(rows))
		if err != nil {
			return err
		}
		for i := range rows {
			assignable, receives := eligible[rows[i].ID], receivers[rows[i].ID]
			rows[i].Assignable, rows[i].ReceivesHandOver = &assignable, &receives
		}
		return nil
	})
}
