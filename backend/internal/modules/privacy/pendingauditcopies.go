// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const historicalAuditPendingSQL = `EXISTS(
 SELECT 1 FROM audit_log history
 WHERE history.entity_type=certificate.entity_type AND history.entity_id=certificate.entity_id
 AND (history.occurred_at,history.id)<=(certificate.occurred_at,certificate.id)
 AND ` + storekit.AuditRedactionNeededSQL + `)
 OR EXISTS(
 SELECT 1 FROM event_outbox retained
 WHERE retained.created_at<=certificate.occurred_at
 AND retained.envelope->'entity'->>'type'=certificate.entity_type
 AND retained.envelope->'entity'->>'id'=certificate.entity_id::text
 AND retained.envelope->'trace'->>'audit_log_id'<>certificate.id::text)`

type HistoricalAuditCursor struct {
	After ids.UUID `json:"after"`
	Until ids.UUID `json:"until"`
	Phase int      `json:"phase"`
}

// RetryHistoricalAuditPage yields after one bounded page, including held rows.
// River keeps the cursor across attempts; the next scheduled job revisits holds.
func (s *RetentionService) RetryHistoricalAuditPage(ctx context.Context, cursor HistoricalAuditCursor) (HistoricalAuditCursor, bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return cursor, false, err
	}
	passes := []func(context.Context, pgx.Tx, ids.UUID, ids.UUID, int, CountryReader) (HistoricalAuditPage, error){
		RedactHistoricalAuditPageTx, RedactLinkedHistoricalAuditPageTx, RedactLinkedHistoricalIdentitiesPageTx,
	}
	if cursor.Phase < 0 || cursor.Phase >= len(passes) {
		return cursor, false, apperrors.ErrInvalidArgument
	}
	if cursor.Until.IsZero() {
		cursor.Until = ids.NewV7()
	}
	var page HistoricalAuditPage
	err := storekit.RetryLockCycles(ctx, func() error {
		return s.db.Tx(ctx, func(tx pgx.Tx) error {
			var err error
			page, err = passes[cursor.Phase](ctx, tx, cursor.After, cursor.Until, retentionBatch, s.eraser.country)
			return err
		})
	})
	if err != nil {
		return cursor, false, err
	}
	if len(page.Scanned) == retentionBatch {
		cursor.After = page.Scanned[len(page.Scanned)-1]
	} else {
		cursor.After = ids.Nil
		cursor.Phase++
	}
	return cursor, cursor.Phase == len(passes), nil
}
