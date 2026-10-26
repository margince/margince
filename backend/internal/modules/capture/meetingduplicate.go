// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CarryMeetingImportsTx records every seat that imported a duplicate copy of a
// meeting as an importer of the copy that survives. A calendar sync arriving
// today records itself the same way on the meeting it binds to. Each seat's
// posture and verdict move with it, so a hold placed on the copy holds on the
// survivor. A seat already recorded on the survivor keeps its own row.
func CarryMeetingImportsTx(ctx context.Context, tx pgx.Tx, duplicate, survivor ids.ActivityID) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO capture_import (activity_id, user_id, posture_at_import, verdict_status, verdict_reason, verdict_reasons,
		                            imported_at, provider_received_at, source_occurred_at, delivered_by, bcc_only)
		SELECT $2, user_id, posture_at_import, verdict_status, verdict_reason, verdict_reasons,
		       imported_at, provider_received_at, source_occurred_at, delivered_by, bcc_only
		  FROM capture_import WHERE activity_id = $1
		ON CONFLICT (activity_id, user_id) DO NOTHING`, duplicate, survivor); err != nil {
		return fmt.Errorf("capture: carrying a duplicate meeting's importers: %w", err)
	}
	return nil
}
