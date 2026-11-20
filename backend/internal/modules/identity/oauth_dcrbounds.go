// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The bounds on the clients that register themselves. Registration needs no
// sign-in, so the rate limit alone would let oauth_client grow for as long as
// someone keeps posting. A cap holds how many never-used ones may exist, and
// the sweep removes them once no sign-in can still be on its way.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// DefaultUnusedDynamicClientCap is far above what real tools make: one client
// per connection a person starts, most of which a consent then uses.
const DefaultUnusedDynamicClientCap = 1000

// DefaultUnusedDynamicClientRetention is how long a self-registered client may
// wait for its first consent. A sign-in runs within minutes of registering; a
// week also covers one a person left open and finishes later.
const DefaultUnusedDynamicClientRetention = 7 * 24 * time.Hour

// unusedDynamicClient is the predicate on an oauth_client aliased c: it
// registered itself and no consent ever made a grant through it. A grant
// pins its client (the foreign key restricts a delete), so a row this matches
// has nothing that could still point at it.
const unusedDynamicClient = `c.created_via = 'dcr'
	AND NOT EXISTS (SELECT 1 FROM oauth_grant g WHERE g.client_id = c.client_id)`

// unusedDynamicClientsFull is what a client refused at the cap reads.
const unusedDynamicClientsFull = "this installation already holds the most self-registered clients that never " +
	"connected; retry once older ones are removed, or ask an admin to register the client"

var errUnusedDynamicClientsFull = fmt.Errorf("%w: %s", apperrors.ErrBudgetExceeded, unusedDynamicClientsFull)

// unusedDynamicClientLock serialises the count and the insert of a
// registration, so two registrations racing at the cap cannot both pass it.
const unusedDynamicClientLock = "identity.oauth_client.dcr_cap"

// WithUnusedDynamicClientCap sets how many never-used self-registered clients
// may exist. Handlers built without it hold DefaultUnusedDynamicClientCap.
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

// insertDynamicClient stores one self-registered client unless the cap on
// never-used ones is reached.
func insertDynamicClient(ctx context.Context, tx pgx.Tx, limit int, clientID string, req dcrRequest) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, unusedDynamicClientLock); err != nil {
		return err
	}
	var unused int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM oauth_client c WHERE `+unusedDynamicClient).Scan(&unused); err != nil {
		return err
	}
	if unused >= limit {
		return errUnusedDynamicClientsFull
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO oauth_client (client_id, client_name, redirect_uris)
		VALUES ($1, $2, $3)`,
		clientID, req.ClientName, req.RedirectURIs)
	return err
}

// PurgeUnusedDynamicClients deletes up to limit self-registered clients that
// no consent used and that registered more than olderThan ago, oldest first,
// and answers how many went. The sweep calls it as the system.
//
// It writes no audit row, as the registration that made the row wrote none.
// Nobody in the workspace acted on either, and no connection ran through it.
func (s *Service) PurgeUnusedDynamicClients(ctx context.Context, olderThan time.Duration, limit int) (int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, err
	}
	var purged int
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// A consent landing meanwhile makes a grant, and the grant's foreign key
		// refuses this delete, so a client someone used is never removed.
		tag, err := tx.Exec(ctx, `
			DELETE FROM oauth_client WHERE id IN (
			       SELECT c.id FROM oauth_client c
			        WHERE `+unusedDynamicClient+` AND c.created_at < now() - make_interval(secs => $1)
			        ORDER BY c.created_at LIMIT $2)`,
			olderThan.Seconds(), limit)
		purged = int(tag.RowsAffected())
		return err
	})
	if err != nil {
		return 0, err
	}
	return purged, nil
}
