// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// BumpDueByMailbox routes a provider push notification onto the sweep: the
// notification names only a mailbox address, and the matching connection is
// found through the provider-owned identity the gmail connector writes into
// its own cursor (sync_cursor->>'email') — no credential is unsealed, no new
// column exists. Matching connections have their pacing clock zeroed so the
// next dispatch picks them up immediately; the returned pairs let the caller
// enqueue the sync jobs directly rather than waiting for the periodic scan.
//
// A mailbox nobody has connected matches nothing and returns empty — a push
// for a foreign address is a no-op, never an error (Pub/Sub redelivers on
// errors, and there is nothing here a retry would fix).
func BumpDueByMailbox(ctx context.Context, pool *pgxpool.Pool, provider, email string) ([]DueConnection, error) {
	return BumpDueByMailboxes(ctx, pool, provider, []string{email})
}

// BumpDueByMailboxes is the same for a DELIVERY naming several mailboxes at
// once, which Graph's batched change notifications do.
//
// One scan serves the whole batch. A per-address loop would make the cost of
// one public request grow with the batch size, which an unauthenticated caller
// could use against the connection pool. Matching a set costs one predicate.
func BumpDueByMailboxes(ctx context.Context, pool *pgxpool.Pool, provider string, emails []string) ([]DueConnection, error) {
	if len(emails) == 0 {
		return nil, nil
	}
	wsID, ok, err := database.LiveWorkspace(ctx, pool)
	if err != nil || !ok {
		return nil, err
	}

	var hits []DueConnection
	wsCtx := principal.WithWorkspaceID(ctx, wsID)
	ws := ids.From[ids.WorkspaceKind](wsID)
	err = database.WithTx(wsCtx, pool, func(tx pgx.Tx) error {
		matched, err := makeDueNowTx(ctx, tx,
			`c.provider = @pushed_provider AND c.status IN ('connected','error') AND c.sync_cursor->>'email' = ANY(@emails)`,
			pgx.NamedArgs{"pushed_provider": provider, "emails": emails})
		if err != nil {
			return err
		}
		for _, id := range matched {
			hits = append(hits, DueConnection{Workspace: ws, ID: id})
		}
		return nil
	})
	if err != nil {
		return hits, fmt.Errorf("capture: push scan: %w", err)
	}
	return hits, nil
}

// makeDueNowTx brings the matching live connections' next sync forward to now
// and returns them. It moves only the clock; the sync it starts writes the outcome.
// An upsert, because a connection that never synced has no sidecar row yet.
// match is a literal predicate over c (capture_connection) and s (capture_sync_state).
func makeDueNowTx(ctx context.Context, tx pgx.Tx, match string, args pgx.NamedArgs) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, `
		INSERT INTO capture_sync_state (connection_id, next_sync_at)
		SELECT c.id, now()
		  FROM capture_connection c
		  LEFT JOIN capture_sync_state s ON s.connection_id = c.id
		 WHERE `+liveConnectionPredicate+` AND `+match+`
		ON CONFLICT (connection_id) DO UPDATE
		   SET next_sync_at = LEAST(capture_sync_state.next_sync_at, now())
		RETURNING connection_id`, args)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}
