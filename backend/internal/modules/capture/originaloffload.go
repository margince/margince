// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/originalstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// OffloadBatch checks legacy originals once, keeping one payload in memory.
func (s *PartSlimStore) OffloadBatch(ctx context.Context, limit int) (considered, offloaded int, err error) {
	if s.blob == nil {
		return 0, 0, nil
	}
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, 0, err
	}
	if limit <= 0 {
		limit = PartSlimBatch
	}
	ctx = originalstore.Bind(ctx, s.db, s.blob)
	var originals []ids.UUID
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id FROM raw_capture
 WHERE payload_offload_checked_at IS NULL AND payload_object IS NULL
 ORDER BY id LIMIT @limit`, pgx.NamedArgs{"limit": limit})
		if err != nil {
			return err
		}
		originals, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		return err
	})
	if err != nil {
		return 0, 0, err
	}
	for _, id := range originals {
		considered++
		moved, err := s.offloadOriginal(ctx, id)
		if err != nil {
			return considered, offloaded, err
		}
		if moved {
			offloaded++
		}
	}
	return considered, offloaded, nil
}

func (s *PartSlimStore) offloadOriginal(ctx context.Context, id ids.UUID) (moved bool, err error) {
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		var payload []byte
		err := tx.QueryRow(ctx, `SELECT payload FROM raw_capture
 WHERE id = @id AND payload_object IS NULL AND payload_offload_checked_at IS NULL
 FOR UPDATE`, pgx.NamedArgs{"id": id}).Scan(&payload)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		inline, ref, err := originalstore.PrepareTx(ctx, tx, payload)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE raw_capture
 SET payload = @payload::jsonb, payload_object = @object::jsonb, payload_offload_checked_at = now()
 WHERE id = @id`, pgx.NamedArgs{"id": id, sqlArgPayload: inline, sqlArgObject: ref}); err != nil {
			return err
		}
		moved = ref != nil
		return originalstore.Adopt(ctx, tx, ref)
	})
	return moved, err
}
