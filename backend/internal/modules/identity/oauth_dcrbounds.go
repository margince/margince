// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The bounds on the clients that make themselves: by registering (DCR) or by
// presenting a metadata document (CIMD). Neither needs a sign-in, so the rate
// limit alone would let oauth_client grow for as long as someone keeps
// posting. A cap holds how many never-used ones may exist, and the sweep
// removes them once no sign-in can still be on its way.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// DefaultUnusedDynamicClientCap is far above what real tools make: one client
// per connection a user starts, most of which a consent then uses.
const DefaultUnusedDynamicClientCap = 1000

// DefaultUnusedDynamicClientRetention is how long a self-made client may wait
// for its first consent. A sign-in runs within minutes of registering, and a
// tool that comes back later registers again.
const DefaultUnusedDynamicClientRetention = 24 * time.Hour

// unusedSelfMadeClient is the predicate on an oauth_client aliased c: it made
// itself and no consent ever made a grant through it. A grant pins its client
// (the foreign key restricts a delete), so a row this matches has nothing that
// could still point at it.
const unusedSelfMadeClient = `c.created_via IN ('dcr', 'cimd')
	AND NOT EXISTS (SELECT 1 FROM oauth_grant g WHERE g.client_id = c.client_id)`

// unusedSelfMadeClientLock serialises the count and the insert of a self-made
// client, so two registrations racing at the cap cannot both pass it.
const unusedSelfMadeClientLock = "identity.oauth_client.self_made_cap"

// evictionReach is how far past the cap one admission looks, so a cap an
// operator lowered is reached again within a few registrations.
const evictionReach = 100

// WithUnusedDynamicClientCap sets how many never-used self-made clients may
// exist. Handlers built without it hold DefaultUnusedDynamicClientCap.
func (h Handlers) WithUnusedDynamicClientCap(limit int) Handlers {
	h.unusedDynamicClientCap = limit
	return h
}

func (h Handlers) unusedDynamicClientLimit() int {
	if h.unusedDynamicClientCap > 0 {
		return h.unusedDynamicClientCap
	}
	return DefaultUnusedDynamicClientCap
}

// makeRoomForSelfMadeClient admits one more self-made client under the cap.
// At the cap it deletes the oldest never-used ones instead of refusing. A real
// tool can then always register, and the table still holds at most the cap.
// A client a consent holds locked is skipped, so the consent wins.
func makeRoomForSelfMadeClient(ctx context.Context, tx pgx.Tx, limit int) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, unusedSelfMadeClientLock); err != nil {
		return err
	}
	var unused int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM oauth_client c WHERE `+unusedSelfMadeClient+`
		LIMIT @reach) reached`, pgx.NamedArgs{"reach": limit + evictionReach}).Scan(&unused); err != nil {
		return err
	}
	if unused < limit {
		return nil
	}
	_, err := deleteUnusedSelfMadeClients(ctx, tx, unused-limit+1, nil)
	return err
}

// deleteUnusedSelfMadeClients deletes up to limit never-used self-made clients,
// oldest first; with olderThan set, only those registered longer ago. A row a
// consent has locked is skipped, and the grant check runs again under the row
// lock. A consent landing at the same moment keeps its client, and the rest
// still go.
func deleteUnusedSelfMadeClients(ctx context.Context, tx pgx.Tx, limit int, olderThan *time.Duration) (int, error) {
	args := pgx.NamedArgs{"limit": limit, "older_secs": nil}
	if olderThan != nil {
		args["older_secs"] = olderThan.Seconds()
	}
	tag, err := tx.Exec(ctx, `
		DELETE FROM oauth_client WHERE id IN (
		       SELECT c.id FROM oauth_client c
		        WHERE `+unusedSelfMadeClient+`
		          AND (@older_secs::float8 IS NULL OR c.created_at < now() - make_interval(secs => @older_secs::float8))
		        ORDER BY c.created_at, c.id LIMIT @limit
		          FOR UPDATE SKIP LOCKED)
		   AND NOT EXISTS (SELECT 1 FROM oauth_grant g WHERE g.client_id = oauth_client.client_id)`, args)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// insertDynamicClient stores one self-registered client, making room under the
// cap first.
func insertDynamicClient(ctx context.Context, tx pgx.Tx, limit int, clientID string, req dcrRequest) error {
	if err := makeRoomForSelfMadeClient(ctx, tx, limit); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO oauth_client (client_id, client_name, redirect_uris)
		VALUES ($1, $2, $3)`,
		clientID, req.ClientName, req.RedirectURIs)
	return err
}

// PurgeUnusedDynamicClients deletes up to limit self-made clients that no
// consent used and that registered more than olderThan ago, oldest first, and
// answers how many went. The sweep calls it as the system.
//
// It writes no audit row, as the registration that made the row wrote none.
// Nobody in the workspace acted on either, and no connection ran through it.
func (s *Service) PurgeUnusedDynamicClients(ctx context.Context, olderThan time.Duration, limit int) (int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, err
	}
	var purged int
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		purged, err = deleteUnusedSelfMadeClients(ctx, tx, limit, &olderThan)
		return err
	})
	if err != nil {
		return 0, err
	}
	return purged, nil
}
