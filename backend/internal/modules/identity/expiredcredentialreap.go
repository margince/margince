// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Every bearer credential this package mints is reaped once it is dead.
//
// Each of these tables grows with traffic and nothing removed a row: a session
// per login, a code per consent, a refresh token per grant, an invite per
// invitation. The sweep rides the write that adds the next one, for the reason
// the spent-nonce ledger gives: a job for rows whose own deadline already says
// when they die is machinery with no tenant, and a login is the moment there is
// certainly another row to pay for it.
//
// Bounded, so one write never carries an unbounded delete. A backlog drains
// over the writes that follow rather than in whichever unlucky request meets
// it.
const expiredCredentialReapLimit = 200

// deadCredentialPredicate is what "dead" means for each table, which is not the
// same sentence twice:
//
//   - a session dies at its absolute deadline or at its idle one, whichever
//     comes first, and a revoked session is dead the moment it is revoked;
//   - an auth_token is dead once used, because the use is one-time;
//   - a consumed setup_token is dead, and it carries no expiry at all, so age
//     since creation is what bounds an unconsumed one.
//
// Spelled per table rather than as one clause: a shared "expires_at < now()"
// keeps every revoked session and every used invite, with nothing to say so.
// hardcoded credential. These are SQL predicates; no secret is spelled here.
//
//nolint:gosec // G101 reads a table name holding "token" beside a string as a
var deadCredentialPredicate = map[string]string{
	"session":                  "revoked_at IS NOT NULL OR expires_at < now() OR idle_expires_at < now()",
	"auth_token":               "used_at IS NOT NULL OR expires_at < now()",
	"oauth_refresh_token":      "expires_at < now()",
	"oauth_authorization_code": "expires_at < now()",
	"setup_token":              "consumed_at IS NOT NULL OR created_at < now() - interval '30 days'",
}

// reapDeadCredentials removes a bounded number of dead rows from one credential
// table, inside the caller's transaction.
//
// The table name is a constant from the map above and never a value off a
// request, so the only identifier formatted into this statement is one this
// package wrote.
func reapDeadCredentials(ctx context.Context, tx pgx.Tx, table string) error {
	dead, known := deadCredentialPredicate[table]
	if !known {
		// A new credential table reaches this through a writer somebody added;
		// saying so beats sweeping nothing and reading as swept.
		return fmt.Errorf("identity: no dead-row rule for credential table %q", table)
	}
	_, err := tx.Exec(ctx,
		`DELETE FROM `+table+` WHERE id IN (SELECT id FROM `+table+
			` WHERE `+dead+` LIMIT $1)`, expiredCredentialReapLimit)
	return err
}
