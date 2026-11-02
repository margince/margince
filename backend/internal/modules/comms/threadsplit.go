// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ThreadSplitDeliveryTx follows one message's grouping without collecting all
// deliveries of a large conversation into one transaction.
func ThreadSplitDeliveryTx(ctx context.Context, tx pgx.Tx, deliveryID ids.UUID, activityID ids.ActivityID, from, to string) error {
	if from == "" || to == "" || from == to {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE comms_outbound SET thread_key=@to
 WHERE id=@delivery_id AND activity_id=@activity AND thread_key=@from`, pgx.NamedArgs{"delivery_id": deliveryID, "activity": activityID, "from": from, "to": to}); err != nil {
		return fmt.Errorf("comms: moving a delivery to its message's conversation: %w", err)
	}
	return nil
}
