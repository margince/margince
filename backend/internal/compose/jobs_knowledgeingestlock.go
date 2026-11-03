// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// holdIngest takes the lock that makes one run of a document's ingest the only
// one, and reports whether this run got it. A false answer means a live run
// holds the document and owns its outcome.
func holdIngest(ctx context.Context, pool *pgxpool.Pool, documentID ids.UUID, log *slog.Logger) (release func(), held bool, err error) {
	return holdSessionLock(ctx, pool, "knowledge_ingest:"+documentID.String(), log)
}
