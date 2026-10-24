// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/mailidentity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// DeliveryReportArchiveReason is the reason the repair archives with.
const DeliveryReportArchiveReason = "delivery_report_filed_as_received_mail"

// DeliveryReportRuleVersion names the sender rule the repair judged with.
const DeliveryReportRuleVersion = "mailidentity.IsDeliverySystemSender"

// importedInboundMailScope selects received mail an importer filed and nobody
// has edited since (version 1). The sender test is not in the SQL: it runs in
// Go, through the one rule the write door uses.
const importedInboundMailScope = `FROM activity a
	 WHERE a.kind = 'email' AND a.direction = 'inbound' AND a.archived_at IS NULL AND a.version = 1
	   AND a.counterparty_email IS NOT NULL AND starts_with(COALESCE(a.source_system, ''), '` + provenance.ReservedSourceSystemPrefix + `')`

// DeliveryReport is a received mail whose sender is the mail system, with the
// leads it is linked to.
type DeliveryReport struct {
	ID    ids.UUID
	Leads []ids.LeadID
}

// DeliveryReportsTx locks the next limit imported received mails after the
// cursor, in id order. It returns every id it inspected, so the cursor advances
// over rows that stay, and the ones from the mail system.
func DeliveryReportsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, []DeliveryReport, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, nil, err
	}
	rows, err := tx.Query(ctx, `SELECT a.id, a.counterparty_email `+importedInboundMailScope+`
		   AND a.id > $1 ORDER BY a.id LIMIT $2 FOR UPDATE OF a`, after, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("activities: reading imported received mail: %w", err)
	}
	inspected, err := pgx.CollectRows(rows, pgx.RowToStructByPos[InboundSender])
	if err != nil {
		return nil, nil, fmt.Errorf("activities: reading imported received mail: %w", err)
	}
	scanned := make([]ids.UUID, 0, len(inspected))
	var reports []DeliveryReport
	for _, mail := range inspected {
		scanned = append(scanned, mail.ID)
		if !mailidentity.IsDeliverySystemSender(mail.Address) {
			continue
		}
		leadRows, err := tx.Query(ctx, `SELECT lead_id FROM activity_link WHERE activity_id = $1 AND lead_id IS NOT NULL`, mail.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("activities: reading a delivery report's leads: %w", err)
		}
		leads, err := pgx.CollectRows(leadRows, pgx.RowTo[ids.LeadID])
		if err != nil {
			return nil, nil, fmt.Errorf("activities: reading a delivery report's leads: %w", err)
		}
		reports = append(reports, DeliveryReport{ID: mail.ID, Leads: leads})
	}
	return scanned, reports, nil
}

// ArchiveDeliveryReportTx archives one delivery report through the module's
// archive, pinned to the version the scan read, so a mail a human edited since
// is refused rather than archived.
func (s *Store) ArchiveDeliveryReportTx(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	version := int64(1)
	return s.ArchiveActivityTx(ctx, tx, ids.From[ids.ActivityKind](id), &version,
		WithArchiveReason(DeliveryReportArchiveReason, DeliveryReportRuleVersion))
}

// CountDeliveryReports counts the imported received mail the repair would
// inspect, reading at most maximum+1. The sender rule runs in Go, so the
// dry run bounds the work and does not state how many rows will change.
func CountDeliveryReports(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	var count int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 `+importedInboundMailScope+` LIMIT $1) imported`, maximum+1).Scan(&count); err != nil {
		return 0, fmt.Errorf("activities: counting imported received mail: %w", err)
	}
	return count, nil
}
