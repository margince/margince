// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// captureWithdrewNote is the ground a case closed by SettleWhenCaptureWithdrewTx
// carries.
const captureWithdrewNote = "Withdrawn by capture: the record was made from mail on its own, a verdict then judged that mail private or noise, and the record was archived rather than processed as business data, so no notice is owed."

// SettleWhenCaptureWithdrewTx ends the open duties of a contact capture made
// and a verdict then withdrew.
//
// A personal thread or a noise sender archives the capture-made contact it
// orphaned (contacts.RetractCaptureOnlyContactTx). The business record that
// would have owed an Art. 14 notice is gone, and a duty left open would put a
// withdrawn record on the worklist with a deadline. Only the duties still owed
// close; a queued disclosure and every ended case keep what they have.
//
// Returns how many cases it closed.
func SettleWhenCaptureWithdrewTx(ctx context.Context, tx pgx.Tx, contactID ids.UUID, now time.Time) (int, error) {
	rows, err := tx.Query(ctx, `
		SELECT id FROM privacy_notice_case
		 WHERE contact_id = $1 AND state = ANY($2)
		 ORDER BY id`, contactID, settleableOwedStates)
	if err != nil {
		return 0, fmt.Errorf("read the duties owed for a withdrawn contact: %w", err)
	}
	caseIDs, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return 0, fmt.Errorf("read the duties owed for a withdrawn contact: %w", err)
	}
	return closeOwedCasesTx(ctx, tx, caseIDs, captureWithdrewNote, now)
}
