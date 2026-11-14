// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// StampReplySettledBatch stamps up to limit inbound messages whose stamp is not
// set, newest first, and reports how many it stamped. A stamp is the earliest
// instant a newer inbound message or our same-thread reply settled the message,
// or infinity when neither has yet. The database clears a stamp whenever a
// write can change it, so the pass runs until it returns zero and again later.
func (s *Store) StampReplySettledBatch(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		return 0, fmt.Errorf("activities: the settled stamp needs a positive batch limit, got %d", limit)
	}
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return 0, err
	}
	var stamped int
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		stamped, err = stampReplySettled(ctx, tx, limit)
		return err
	})
	return stamped, err
}

// stampReplySettled locks the batch before it reads the threads. A write that
// would clear a stamp waits on that lock, so it clears what this pass wrote. A
// write that committed first is in the snapshot the read takes. The database
// writes the stamps, as bookkeeping that moves no version.
func stampReplySettled(ctx context.Context, tx pgx.Tx, limit int) (int, error) {
	rows, err := tx.Query(ctx, `
		SELECT id FROM activity
		 WHERE reply_settled_at IS NULL AND direction = 'inbound' AND kind IN ('email', 'message')
		   AND restricted_at IS NULL
		 ORDER BY occurred_at DESC, id
		 LIMIT $1
		   FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return 0, fmt.Errorf("activities: choosing messages to stamp: %w", err)
	}
	batch, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil || len(batch) == 0 {
		return 0, err
	}
	read, err := tx.Query(ctx, `
		SELECT a.id, coalesce(least(
		         (SELECT min(newer.occurred_at) FROM activity newer WHERE `+newerInboundSQL("'infinity'::timestamptz")+`),
		         `+storekit.ThreadAnsweredAtSQL("a")+`), 'infinity')
		  FROM activity a WHERE a.id = ANY($1)`, batch)
	if err != nil {
		return 0, fmt.Errorf("activities: reading when messages were settled: %w", err)
	}
	var messages []ids.UUID
	var settled []pgtype.Timestamptz
	var message ids.UUID
	var at pgtype.Timestamptz
	if _, err := pgx.ForEachRow(read, []any{&message, &at}, func() error {
		messages, settled = append(messages, message), append(settled, at)
		return nil
	}); err != nil {
		return 0, fmt.Errorf("activities: reading when messages were settled: %w", err)
	}
	var kept int
	if err := tx.QueryRow(ctx, `SELECT keep_reply_settled($1, $2)`, messages, settled).Scan(&kept); err != nil {
		return 0, fmt.Errorf("activities: stamping when messages were settled: %w", err)
	}
	return kept, nil
}
