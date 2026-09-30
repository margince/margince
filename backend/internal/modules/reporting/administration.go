// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PauseAll disables schedules without removing their retained editions.
func (s *Service) PauseAll(ctx context.Context) (int64, error) {
	if err := requireReadWrite(ctx, "reporting_framework", principal.ActionUpdate); err != nil {
		return 0, err
	}
	var count int64
	err := s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := s.authority.Scope(ctx, tx, crmcontracts.ReportingScope{Kind: reportingWorkspace}, true); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, "SELECT "+scheduleColumns+" FROM report_schedule WHERE enabled ORDER BY report_id,id FOR UPDATE")
		if err != nil {
			return err
		}
		schedules, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (crmcontracts.ReportingSchedule, error) { return scanSchedule(row) })
		if err != nil {
			return err
		}
		for _, schedule := range schedules {
			if err := s.suspendSchedule(ctx, tx, schedule, "Paused by the reporting administrator; explicit resume required"); err != nil {
				return err
			}
			count++
		}
		return nil
	})
	return count, err
}
