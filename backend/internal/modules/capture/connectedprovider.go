// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Whether a contact's account on some provider actually reaches this product.
//
// It is a read of capture's own table by a caller that is not capture — the
// scheduling seam, which has to know whether a calendar backs a free/busy
// answer before it publishes one. That question has no home in activities (it
// owns no connection) and none in the transport, so it lives beside the rows
// that answer it and compose carries it across (ADR-0054 §9).

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ConnectedOnAny reports whether this user holds a LIVE connection on any of
// the named providers.
//
// Live means `connected`: a connection in error or awaiting re-consent has a
// standing grant but is not delivering, so whatever it would have carried is
// missing from the product exactly as an absent connection's would be. A caller
// asking this is about to decide how much to claim for an answer, and the two
// ways of being wrong are not symmetrical — under-claiming costs some caution,
// over-claiming is the empty-diary report this was written for.
//
// An empty provider list is false rather than an error: no provider can be
// connected, which is the answer, and a caller that derives its list from a
// vocabulary should not have to handle a second outcome when the vocabulary is
// short.
func ConnectedOnAny(ctx context.Context, tx pgx.Tx, user ids.UserID, providers []string) (bool, error) {
	if len(providers) == 0 {
		return false, nil
	}
	var connected bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM capture_connection
		  WHERE user_id = $1 AND provider = ANY($2) AND status = 'connected'
		    AND archived_at IS NULL)`, user, providers).Scan(&connected)
	if err != nil {
		return false, fmt.Errorf("capture: reading this user's live connections: %w", err)
	}
	return connected, nil
}
