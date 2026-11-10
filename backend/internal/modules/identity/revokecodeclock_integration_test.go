// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A code issued while a revocation is under way is still ended by it.
//
// The revocation's transaction began first, so its now() is earlier than the
// code's created_at. Ending the code at now() would date its expiry before its
// creation, which the lifetime check refuses. The whole revocation would then
// roll back with the code still usable.
func TestARevocationEndsACodeIssuedAfterItBegan(t *testing.T) {
	e := setupRevocationEnv(t, "revoke-code-clock")
	ctx := e.wsCtx(e.member)
	codeID := ids.NewV7()

	err := e.svc.db.Tx(ctx, func(tx pgx.Tx) error {
		// The revocation's own now(), fixed when its transaction began.
		var began time.Time
		if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&began); err != nil {
			return err
		}
		// Issued and committed on another connection, after the revocation began.
		var created time.Time
		if err := e.owner.QueryRow(context.Background(), `INSERT INTO oauth_authorization_code
			(id, client_id, code_hash, code_challenge, redirect_uri, scopes, user_id, expires_at)
			VALUES ($1, 'client-under-test', $2, 'challenge', 'https://client.test/cb', '{}', $3,
			        now() + interval '10 minutes')
			RETURNING created_at`,
			codeID, "hash-"+codeID.String(), e.member.UserID).Scan(&created); err != nil {
			t.Fatalf("seed the code issued mid-revocation: %v", err)
		}
		if !created.After(began) {
			t.Fatalf("the code was created at %v, not after the revocation began at %v; the case is not staged", created, began)
		}
		return endCredentialAuthority(selfActorCtx(ctx, e.member.UserID), tx, e.member.UserID, passwordResetRevokeReason)
	})
	if err != nil {
		t.Fatalf("the revocation refused a code issued after it began: %v", err)
	}

	var live bool
	if err := e.owner.QueryRow(context.Background(),
		`SELECT expires_at > now() FROM oauth_authorization_code WHERE id = $1`, codeID).Scan(&live); err != nil {
		t.Fatal(err)
	}
	if live {
		t.Error("the code issued during the revocation is still open")
	}
}
