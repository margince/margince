// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// PurgeActivityProjectionsTx invalidates content-derived state before erasure can remove its source.
func (s *PendingStore) PurgeActivityProjectionsTx(ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID) error {
	if len(activityIDs) == 0 {
		return nil
	}
	if err := purgeThreadContentTx(ctx, tx, activityIDs); err != nil {
		return err
	}
	return s.PurgeRawCaptureTx(ctx, tx, activityIDs)
}

// PurgeRawCaptureTx retires provider originals and their recipient proof. Retained
// activity content still supports its subject graph when only the original expires.
func (s *PendingStore) PurgeRawCaptureTx(ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID) error {
	if len(activityIDs) == 0 {
		return nil
	}
	// The stored link covers every provider; natural-key matching also reaches older mail.
	if _, err := tx.Exec(ctx, `DELETE FROM raw_capture r USING activity a
 WHERE a.id=ANY($1) AND `+storekit.StoredOriginalSQL("a", "r"), activityIDs); err != nil {
		return fmt.Errorf("capture: purging provider originals: %w", err)
	}
	return purgePartySourcesTx(ctx, tx, activityIDs)
}
