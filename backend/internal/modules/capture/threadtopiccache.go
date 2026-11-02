// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Surviving messages keep their own proof while their inherited comparison is recomputed.
func invalidateThreadTopicCacheTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID) error {
	_, err := tx.Exec(ctx, `UPDATE capture_thread_topic SET effective_topic='' WHERE activity_id=@id`, pgx.NamedArgs{"id": id})
	return err
}
