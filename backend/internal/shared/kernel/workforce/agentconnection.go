// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package workforce

// LiveOAuthClientSQL is the predicate on an oauth_client aliased c: an admin has
// neither disabled nor deleted the tool.
const LiveOAuthClientSQL = `c.disabled_at IS NULL AND c.deleted_at IS NULL`

// LiveSetterJoins reaches the connection behind a passport aliased p.
const LiveSetterJoins = `
	LEFT JOIN oauth_grant g ON g.id = p.oauth_grant_id
	LEFT JOIN oauth_client c ON c.client_id = g.client_id`

// UnrevokedSetterSQL is whether nobody ended the authority behind passport p. A
// connected passport answers through its connection, because a refresh retires the
// passport and leaves the same agent on a fresher one. A passport with no grant
// answers for itself. It reads over LiveSetterJoins.
const UnrevokedSetterSQL = `(CASE WHEN p.oauth_grant_id IS NULL THEN p.revoked_at IS NULL
	ELSE coalesce(g.revoked_at IS NULL AND c.client_id IS NOT NULL AND ` + LiveOAuthClientSQL + `, false) END)`

// LiveSetterSQL is whether the agent behind passport p still stands behind work it
// left running, as of the SQL expression at. It is UnrevokedSetterSQL and not yet
// run out. A passport with no grant runs out with its own expiry. A connection runs
// out only when it holds no unexpired passport and can mint none. So an access token
// that lapsed on a grant that can still refresh is not dead.
func LiveSetterSQL(at string) string {
	return `(` + UnrevokedSetterSQL + ` AND (CASE WHEN p.oauth_grant_id IS NULL THEN ` + at + ` < p.expires_at
	ELSE EXISTS (SELECT 1 FROM passport q WHERE q.oauth_grant_id = g.id AND q.revoked_at IS NULL AND ` + at + ` < q.expires_at)
	  OR (g.refresh_allowed AND EXISTS (SELECT 1 FROM oauth_refresh_token r
	       WHERE r.grant_id = g.id AND r.consumed_at IS NULL AND ` + at + ` < r.expires_at)) END))`
}
