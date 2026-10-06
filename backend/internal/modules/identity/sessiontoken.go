// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The session token itself: how one is minted, how it is stored, and how a
// presented one is reduced to the value the row holds. Split out of service.go
// because it is one concept with one rule — the raw token exists only in the
// response, and only its hash is ever written.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/bearer"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func insertSession(ctx context.Context, tx pgx.Tx, userID ids.UserID, tokenHash string) error {
	// The device rides the context, not this signature: it is a transport
	// detail the login handler captured, absent (NULL) for any session opened
	// without a request behind it. See withUserAgent.
	_, err := tx.Exec(ctx,
		`INSERT INTO session (user_id, token_hash, user_agent, idle_expires_at, expires_at)
		 VALUES ($1, $2, $3, now() + $4::interval, now() + $5::interval)`,
		userID, tokenHash, userAgentFromContext(ctx), idleTTL.String(), absoluteTTL.String())
	return err
}

// mintSessionToken returns the raw cookie value and the SHA-256 hex the
// database stores — the raw token never touches the DB (ADR-0043).
//
// Through the shared primitive rather than spelled here: every bearer
// capability in this tree obeys the same rule, and a second spelling of the
// mint is where two of them come to disagree about what gets stored.
func mintSessionToken() (raw, hash string, err error) {
	return bearer.Mint()
}

func hashToken(raw string) string { return bearer.Digest(raw) }
