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
	if _, err := tx.Exec(ctx, `
		INSERT INTO capture_import (activity_id, user_id, posture_at_import, verdict_status, verdict_reason, verdict_reasons,
		                            imported_at, provider_received_at, source_occurred_at, delivered_by, bcc_only, provider_message_ids)
		SELECT $2, user_id, posture_at_import, verdict_status, verdict_reason, verdict_reasons,
		       imported_at, provider_received_at, source_occurred_at, delivered_by, bcc_only, provider_message_ids
		  FROM capture_import WHERE activity_id = $1 AND user_id = $3
		ON CONFLICT (activity_id, user_id) DO UPDATE SET
 provider_message_ids=ARRAY(SELECT DISTINCT unnest(capture_import.provider_message_ids || EXCLUDED.provider_message_ids) ORDER BY 1),
 delivered_by=ARRAY(SELECT DISTINCT unnest(capture_import.delivered_by || EXCLUDED.delivered_by) ORDER BY 1)
 WHERE NOT EXCLUDED.provider_message_ids <@ capture_import.provider_message_ids OR NOT EXCLUDED.delivered_by <@ capture_import.delivered_by`, duplicate, survivor, seat); err != nil {
		return fmt.Errorf("capture: carrying a duplicate activity's importer: %w", err)
	}
	return nil
}
