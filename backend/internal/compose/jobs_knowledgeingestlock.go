// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// knowledgeIngestUnlockWait bounds the unlock that ends a run, which has to
// happen after the run's own context may already be cancelled.
const knowledgeIngestUnlockWait = 5 * time.Second

// holdIngest takes the lock that makes one run of a document's ingest the only
// one, and reports whether this run got it. A false answer means a live run
// holds the document and owns its outcome.
//
// A session lock, because an ingest is several transactions with long model
// calls between them. The server frees it when the holding connection ends, so
// a killed worker needs no cleanup before the next run can start.
func holdIngest(ctx context.Context, pool *pgxpool.Pool, documentID ids.UUID, log *slog.Logger) (release func(), held bool, err error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("taking a connection for the ingest lock: %w", err)
	}
	key := "knowledge_ingest:" + documentID.String()
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1, 0))`, key).Scan(&held); err != nil {
		conn.Release()
		return nil, false, fmt.Errorf("taking the ingest lock: %w", err)
	}
	if !held {
		conn.Release()
		return nil, false, nil
	}
	return func() {
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), knowledgeIngestUnlockWait)
		defer cancel()
		var unlocked bool
		if err := conn.QueryRow(unlockCtx, `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, key).Scan(&unlocked); err != nil || !unlocked {
			// A connection that may still hold the lock is closed, never
			// returned to the pool: the next borrower would carry the lock.
			log.WarnContext(ctx, "the ingest lock could not be released; closing its connection", "document_id", documentID, "err", err)
			if cerr := conn.Hijack().Close(unlockCtx); cerr != nil {
				log.WarnContext(ctx, "closing the connection that held the ingest lock", "document_id", documentID, "err", cerr)
			}
			return
		}
		conn.Release()
	}, true, nil
}
