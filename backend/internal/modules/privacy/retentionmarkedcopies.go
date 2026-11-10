// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (s *RetentionService) retryMarkedAuditCopies(ctx context.Context) error {
	const pageSize = MarkerAuditPageSize
	until := ids.NewV7()
	for _, correct := range []func(context.Context, pgx.Tx, ids.UUID, ids.UUID, int, CountryReader) (HistoricalAuditPage, error){RedactMarkedWeeklyAuditPageTx, RedactRetiredRequestAuditPageTx} {
		for {
			var page HistoricalAuditPage
			err := storekit.RetryLockCycles(ctx, func() error {
				return s.db.Tx(ctx, func(tx pgx.Tx) error {
					var err error
					page, err = correct(ctx, tx, ids.UUID{}, until, pageSize, s.eraser.country)
					return err
				})
			})
			if err != nil {
				return err
			}
			if page.Changed == 0 || len(page.Scanned) < pageSize {
				break
			}
		}
	}
	return nil
}
