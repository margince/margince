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

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func insertSession(ctx context.Context, tx pgx.Tx, userID ids.UserID, tokenHash string, providerAuthenticated bool) error {
	// The device and address ride the context, not this signature: the login
	// handlers bind them, and a session opened with no request records NULL.
	origin := httpserver.RequestOriginFrom(ctx)
	if err := reapDeadCredentials(ctx, tx, "session"); err != nil {
		return err
	}
	_, err := tx.Exec(ctx,
		`INSERT INTO session (user_id, token_hash, user_agent, ip, idle_expires_at, expires_at, provider_authenticated)
   VALUES (@user_id, @token, @agent, @ip::inet, now() + @idle::interval, now() + @absolute::interval, @provider)`,
		pgx.StrictNamedArgs{
			"user_id": userID, "token": tokenHash, "agent": origin.UserAgent, "ip": origin.IP,
			"idle": idleTTL.String(), "absolute": absoluteTTL.String(), "provider": providerAuthenticated,
		})
	return err
}

// mintSessionToken returns the raw cookie value and the hash the database
// stores. The raw token never reaches the database (ADR-0043).
func mintSessionToken() (raw, hash string, err error) {
	return storekit.MintToken()
}

func hashToken(raw string) string { return storekit.TokenHash(raw) }
