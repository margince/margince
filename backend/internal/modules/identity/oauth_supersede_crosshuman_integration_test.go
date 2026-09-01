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
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

// TestTwoConsentsRacingTheSameRegistrationNeverLeaveTwoActiveGrants proves
// lockClientRegistration (oauth_grant.go) does what its comment claims: without
// it, two issueGrant calls for the same client_id can each read
// supersedePriorGrants' "no active grant yet" snapshot before either commits,
// and both succeed — two live grants for one registration, which the file
// header's invariant forbids. Run repeatedly because the interleaving that
// would expose a missing lock is the scheduler's choice, not ours (same
// reasoning as TestARevokeRacingARotationNeverDeadlocksOrLeavesACredentialLive,
// oauth_lockorder_integration_test.go).
func TestTwoConsentsRacingTheSameRegistrationNeverLeaveTwoActiveGrants(t *testing.T) {
	e := setupRevocationEnv(t, "oauth-supersede-race")

	const attempts = 12
	for attempt := range attempts {
		clientID := "client-" + ids.NewV7().String()
		if _, err := e.owner.Exec(context.Background(), `
			INSERT INTO oauth_client (client_id, client_name, redirect_uris)
			VALUES ($1, 'race client', ARRAY['https://client.example/cb'])`, clientID); err != nil {
			t.Fatalf("attempt %d: registering the client: %v", attempt, err)
		}

		var (
			wg                  sync.WaitGroup
			adminErr, memberErr error
			adminGrant          ids.UUID
			memberGrant         ids.UUID
		)
		wg.Add(2)
		go func() {
			defer wg.Done()
			adminGrant, adminErr = issueGrantTx(e, e.admin, clientID)
		}()
		go func() {
			defer wg.Done()
			memberGrant, memberErr = issueGrantTx(e, e.member, clientID)
		}()
		wg.Wait()

		if adminErr != nil {
			t.Fatalf("attempt %d: the admin's consent failed on the interleaving: %v", attempt, adminErr)
		}
		if memberErr != nil {
			t.Fatalf("attempt %d: the member's consent failed on the interleaving: %v", attempt, memberErr)
		}

		var active int
		if err := e.owner.QueryRow(context.Background(),
			`SELECT count(*) FROM oauth_grant WHERE client_id = $1 AND revoked_at IS NULL`,
			clientID).Scan(&active); err != nil {
			t.Fatalf("attempt %d: counting active grants: %v", attempt, err)
		}
		if active != 1 {
			t.Fatalf("attempt %d: %d active grants for one registration, want 1 (admin=%s member=%s)",
				attempt, active, adminGrant, memberGrant)
		}
	}
}

// issueGrantTx runs issueGrant in its own transaction for the given consenter,
// the shape a real consent commits in.
func issueGrantTx(e *revocationEnv, consenter Identity, clientID string) (grantID ids.UUID, err error) {
	ctx := e.wsCtx(consenter)
	err = e.svc.db.Tx(ctx, func(tx pgx.Tx) error {
		var txErr error
		grantID, _, txErr = issueGrant(ctx, tx, issueGrantInput{
			WorkspaceID: consenter.WorkspaceID, UserID: consenter.UserID, ClientID: clientID,
			Scopes: []string{"read"}, RefreshAllowed: true,
		})
		return txErr
	})
	return grantID, err
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
