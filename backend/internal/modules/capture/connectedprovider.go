// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Whether a contact's account on some provider actually reaches this product.
//
// It is a read of capture's own tables by callers that are not capture — the
// scheduling seam, which has to know whether a calendar backs a free/busy
// answer before it publishes one, and the quiet-record scan, which has to know
// whether a silent timeline means silence. The question has no home in
// activities (it owns no connection) and none in the transport, so it lives
// beside the rows that answer it and compose carries it across (ADR-0054 §9).

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
		  SELECT 1 FROM capture_connection c
		  WHERE c.user_id = $1 AND c.provider = ANY($2) AND `+liveConnection("c")+`)`,
		user, providers).Scan(&connected)
	if err != nil {
		return false, fmt.Errorf("capture: reading this user's live connections: %w", err)
	}
	return connected, nil
}

// liveConnection is what "live" means for one capture_connection alias, in
// SQL — see ConnectedOnAny for why an errored connection is not live. Both
// ConnectedOnAny and MailboxCaughtUpSQL read it.
func liveConnection(alias string) string {
	return fmt.Sprintf("%[1]s.status = 'connected' AND %[1]s.archived_at IS NULL", alias)
}

// MailboxCaughtUpSQL renders "Margince can see this seat's mail" for a SQL
// expression naming a user id, with the mail providers bound at providersPos:
// the seat holds a live connection on one of them, and none of its live ones
// is still importing its history.
//
// A running import is excluded because until it finishes the timeline is
// missing exactly the older mail it is pulling in, so a record the seat wrote
// to last week can read as silent. compose hands this to the quiet-record
// scan in activities, which cannot import this package.
//
// userExpr is formatted in and must be a compile-time SQL expression from the
// caller's own query, never input.
func MailboxCaughtUpSQL(userExpr string, providersPos int) string {
	return fmt.Sprintf(`(EXISTS (
		    SELECT 1 FROM capture_connection seen_conn
		    WHERE seen_conn.user_id = %[1]s AND seen_conn.provider = ANY($%[2]d)
		      AND %[3]s)
		  AND NOT EXISTS (
		    SELECT 1 FROM capture_connection seen_conn
		    JOIN capture_backfill seen_fill ON seen_fill.connection_id = seen_conn.id
		                                   AND seen_fill.status IN ('queued', 'running')
		    WHERE seen_conn.user_id = %[1]s AND seen_conn.provider = ANY($%[2]d)
		      AND %[3]s))`,
		userExpr, providersPos, liveConnection("seen_conn"))
}
