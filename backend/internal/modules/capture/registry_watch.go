// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The Gmail Pub/Sub push-watch lifecycle for the persisted connectors: the
// fleet-wide scan that finds connections whose watch is missing or nearing
// expiry (DueWatches), and the per-connection register/renew that stores the
// new deadline in capture_connection.watch_expires_at (RenewWatch). It reads/
// writes only watch_expires_at (CAP-DDL-2, idx_capture_watch_renew); the
// sync_cursor stays owned by SyncOnce (registry.go). The register+renew
// mechanics live here; the connect/sync/disconnect lifecycle is in registry.go
// and registry_connections.go.
//
// This is only the watch subscription's renewal, not its consumption: the
// public Pub/Sub push webhook that turns a notification back into a sync is a
// separate, security-sensitive surface (CAP-WIRE-N-1; EP05.4a/.8) and is
// composed elsewhere — it is token-gated, and OIDC-verified when the deployment
// configures the push identity. A registered watch therefore drives real syncs;
// the poll (DueConnections → SyncOnce) is the safety net behind it, not the
// latency floor.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// DueWatches lists every connected connection for provider name across the
// whole fleet whose push watch needs (re)registering — either it has none yet
// (watch_expires_at IS NULL) or it expires within the renewal window. It walks
// each workspace under its own GUC, the same RLS fleet-walk shape as
// DueConnections; the near-expiry branch is served by idx_capture_watch_renew.
// One workspace's failure does not starve the rest.
func (r *Registry) DueWatches(ctx context.Context, name string, within time.Duration) ([]DueConnection, error) {
	threshold := time.Now().Add(within)
	return r.collectDue(ctx, func(ctx context.Context, tx pgx.Tx) ([]ids.UUID, error) {
		// The near-expiry branch is served by idx_capture_watch_renew; a
		// never-watched row (watch_expires_at IS NULL) is due for an initial
		// registration.
		rows, err := tx.Query(ctx, `
			SELECT id FROM capture_connection
			WHERE provider = $1 AND status = 'connected' AND archived_at IS NULL
			  AND (watch_expires_at IS NULL OR watch_expires_at < $2)`, name, threshold)
		if err != nil {
			return nil, err
		}
		return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	})
}

// RenewWatch registers (or renews) the push watch for one connection against
// topic and stores the returned expiration in watch_expires_at. It resolves the
// credential and builds the connector principal exactly like SyncOnce, then
// calls the connector's Watcher seam. It deliberately does NOT touch
// sync_cursor: the watch's historyId would suppress the first-sync backfill on
// a fresh connection, so the cursor stays SyncOnce's. The status guard keeps a
// watch that races a concurrent Disconnect from writing a deadline onto a
// no-longer-connected row.
func (r *Registry) RenewWatch(ctx context.Context, connectionID ids.UUID, topic string) error {
	var (
		name          string
		grantedBy     ids.UserID
		credentialRef *string
		authBytes     []byte
		watchRef      *string
		generation    int64
	)
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT provider, user_id, credential_ref, auth, watch_ref, generation FROM capture_connection
			WHERE id = $1 AND status = 'connected'`, connectionID).
			Scan(&name, &grantedBy, &credentialRef, &authBytes, &watchRef, &generation)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("capture: connection %s: %w", connectionID, apperrors.ErrNotFound)
	}
	if err != nil {
		return err
	}
	c, err := r.connector(name)
	if err != nil {
		return err
	}
	watcher, ok := c.(connector.Watcher)
	if !ok {
		return fmt.Errorf("capture: connector %q does not support push-watch renewal", name)
	}
	auth, err := r.resolveCredential(ctx, credentialRef, authBytes)
	if err != nil {
		return err
	}
	runCtx, err := r.connectorContext(ctx, name, grantedBy)
	if err != nil {
		return err
	}
	// BY THE HANDLE, where there is one and the connector renews that way.
	//
	// Watch has to make sure a subscription exists, and a provider that issues
	// handles cannot answer that without first finding the one it made — a
	// paged listing, once per mailbox, every cycle. RenewWatch asks the question
	// a renewal actually has: extend THIS one.
	//
	// The fallback is not a degraded path, it is the first registration and
	// every connection made before a handle was stored. It is also the recovery
	// path, which is where a listing belongs.
	res, err := renewOrRegisterWatch(runCtx, c, watcher, auth, topic, watchRef)
	if err != nil {
		return err
	}
	// FENCED ON THE GENERATION THE READ SAW. The connector call above is a round
	// trip, and a rebind can commit inside it — pointing the row at a different
	// mailbox and clearing the handle on the way past. Unfenced, this write puts
	// the OLD mailbox's subscription back, which is the reset undone by a
	// renewal that was already in flight when it happened. A rebind bumps the
	// generation, so a stale renewal matches nothing and writes nothing; the
	// scan picks the row up again on its own with no handle to renew by, and
	// registers.
	return r.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE capture_connection SET watch_expires_at = $2, watch_ref = NULLIF($3, '')
			WHERE id = $1 AND status = 'connected' AND archived_at IS NULL AND generation = $4`,
			connectionID, res.ExpiresAt, res.Ref, generation)
		return err
	})
}

// renewOrRegisterWatch extends the subscription this connection already holds,
// or registers one when it holds none.
func renewOrRegisterWatch(
	ctx context.Context, c connector.Connector, watcher connector.Watcher,
	auth connector.Auth, topic string, ref *string,
) (connector.WatchResult, error) {
	renewer, renews := c.(connector.WatchRenewer)
	if !renews || ref == nil || *ref == "" {
		return watcher.Watch(ctx, auth, topic)
	}
	return renewer.RenewWatch(ctx, auth, topic, *ref)
}
