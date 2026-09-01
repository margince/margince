// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// supersedePriorGrants ends the REGISTRATION's earlier connection, not only
// the consenting human's own — a client_id re-authorized by a second human
// (a shared machine, a handed-off install) is still one registration
// changing hands, per the file header's stated invariant in oauth_grant.go.
// This is the case the same-human tests next door (oauth_supersede_
// integration_test.go, package agentaccess) cannot cover: they always
// reconsent as the same identity.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestReconsentByADifferentHumanSupersedesTheSameRegistration(t *testing.T) {
	e := setupRevocationEnv(t, "oauth-supersede-cross-human")

	first := e.connectOAuthFor(t, e.admin)
	second := e.connectOAuthWithClient(t, e.member, first.clientID)

	var firstRevokedAt, secondRevokedAt *string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT revoked_at::text FROM oauth_grant WHERE id = $1`, first.grantID).Scan(&firstRevokedAt); err != nil {
		t.Fatalf("reading the first grant: %v", err)
	}
	if err := e.owner.QueryRow(context.Background(),
		`SELECT revoked_at::text FROM oauth_grant WHERE id = $1`, second.grantID).Scan(&secondRevokedAt); err != nil {
		t.Fatalf("reading the second grant: %v", err)
	}
	if firstRevokedAt == nil {
		t.Fatal("the first human's grant must be revoked once a second human re-authorizes the same client registration")
	}
	if secondRevokedAt != nil {
		t.Fatal("the second human's grant is the live connection and must not be touched")
	}
}

// connectOAuthWithClient is connectOAuthWith without minting a NEW client_id:
// the point of this test is two different humans authorizing the SAME
// registration, so the client row already exists from the first call.
func (e *revocationEnv) connectOAuthWithClient(t *testing.T, consenter Identity, clientID string) connectFixture {
	t.Helper()
	var out connectFixture
	out.clientID = clientID
	ctx := e.wsCtx(consenter)
	if err := e.svc.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out.grantID, out.refresh, err = issueGrant(ctx, tx, issueGrantInput{
			WorkspaceID: consenter.WorkspaceID, UserID: consenter.UserID, ClientID: clientID,
			Scopes: []string{"read"}, RefreshAllowed: true,
		})
		return err
	}); err != nil {
		t.Fatalf("issuing the second grant: %v", err)
	}
	return out
}
