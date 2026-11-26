// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CarryImportTx preserves one proven contributor, including their sharing holds.
func CarryImportTx(ctx context.Context, tx pgx.Tx, duplicate, survivor ids.ActivityID, seat ids.UUID) error {
	return carryImporterTx(ctx, tx, duplicate, survivor, seatImporter(seat))
}

// CarrySharedImportTx is CarryImportTx for a shared mailbox's import row.
func CarrySharedImportTx(ctx context.Context, tx pgx.Tx, duplicate, survivor ids.ActivityID, connection ids.UUID) error {
	return carryImporterTx(ctx, tx, duplicate, survivor, mailboxImporter(connection))
}

func carryImporterTx(ctx context.Context, tx pgx.Tx, duplicate, survivor ids.ActivityID, who importer) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO capture_import (activity_id, `+who.column+`, posture_at_import, verdict_status, verdict_reason, verdict_reasons,
		                            imported_at, provider_received_at, source_occurred_at, delivered_by, bcc_only, provider_message_ids, legal_entity_id)
		SELECT $2, `+who.column+`, posture_at_import, verdict_status, verdict_reason, verdict_reasons,
		       imported_at, provider_received_at, source_occurred_at, delivered_by, bcc_only, provider_message_ids, legal_entity_id
		  FROM capture_import WHERE activity_id = $1 AND `+who.column+` = $3
		ON CONFLICT `+who.conflict+` DO UPDATE SET
 provider_message_ids=ARRAY(SELECT DISTINCT unnest(capture_import.provider_message_ids || EXCLUDED.provider_message_ids) ORDER BY 1),
 delivered_by=ARRAY(SELECT DISTINCT unnest(capture_import.delivered_by || EXCLUDED.delivered_by) ORDER BY 1)
 WHERE NOT EXCLUDED.provider_message_ids <@ capture_import.provider_message_ids OR NOT EXCLUDED.delivered_by <@ capture_import.delivered_by`, duplicate, survivor, who.id); err != nil {
		return fmt.Errorf("capture: carrying a duplicate activity's importer: %w", err)
	}
	return nil
}
