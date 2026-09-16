// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// supersedePriorGrants ends the earlier connection of ONE HUMAN on one client
// registration, and never another human's, per the invariant stated in
// oauth_grant.go. This is the case the same-human tests next door
// (oauth_supersede_integration_test.go, package agentaccess) cannot cover:
// they always reconsent as the same identity, so nothing there can observe a
// second human's seat being taken away.
//
// The seat has to be per human because a client_id no longer names one
// install. Under CIMD (oauth_cimd.go) it is a metadata-document URL published
// by the client's author, identical for every human running that software, so
// "one live grant per client_id" would mean one live connection per
// INSTALLATION — every colleague evicting the last one, through the same
// cascade that answers token theft.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestReconsentByADifferentHumanLeavesTheFirstHumansConnectionAlive(t *testing.T) {
	e := setupRevocationEnv(t, "oauth-supersede-cross-human")

	first := e.connectOAuthFor(t, e.admin)
	second := e.connectOAuthWithClient(t, e.member, first.clientID)

	if !e.grantLive(t, first.grantID) {
		t.Fatal("a second human consenting to the same client registration must not revoke the first human's grant")
	}
	if !e.grantLive(t, second.grantID) {
		t.Fatal("the consenting human's own grant is live")
	}
}

// TestTwoHumansOnOneCIMDRegistrationKeepSeparateSeats is the shape the
// deployed product actually meets, named as itself so the next reader does not
// have to derive it from the DCR fixtures every other test here uses: one
// metadata-document client_id, published by the client's author, authorized by
// two unrelated colleagues. Both connections are theirs and neither consent is
// news to the other.
func TestTwoHumansOnOneCIMDRegistrationKeepSeparateSeats(t *testing.T) {
	e := setupRevocationEnv(t, "oauth-supersede-cimd")

	// An https URL with a path, which is what makes an id a CIMD id, and the
	// provenance an admin would see beside it.
	clientID := fmt.Sprintf("https://client.example/%s/oauth-client-metadata", ids.NewV7())
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO oauth_client (client_id, client_name, redirect_uris, created_via)
		VALUES ($1, 'metadata-document client', ARRAY['https://client.example/cb'], 'cimd')`,
		clientID); err != nil {
		t.Fatalf("recording the metadata-document client: %v", err)
	}

	firstHuman := e.connectOAuthWithClient(t, e.admin, clientID)
	secondHuman := e.connectOAuthWithClient(t, e.member, clientID)

	if !e.grantLive(t, firstHuman.grantID) {
		t.Fatal("a colleague connecting the same published client must not end this human's connection")
	}
	if !e.grantLive(t, secondHuman.grantID) {
		t.Fatal("the second human's own connection is live")
	}
}

// TestTwoConsentsRacingOneHumansSeatNeverLeaveTwoActiveGrants proves the
// serialization issueGrant relies on still holds where the seat now lives.
// Without it, two consents from ONE human can each read supersedePriorGrants'
// "no active grant yet" snapshot before either commits, and both succeed —
// two live grants on one seat, which the invariant forbids and which would put
// a second row in that human's Settings for every reconnect.
//
// The serialization point is requireLiveConsentingUser's app_user FOR UPDATE,
// taken immediately before the supersede: a seat is (registration, human), so
// the human's own row is what two consents for it queue on. Run repeatedly
// because the interleaving that would expose its absence is the scheduler's
// choice, not ours (same reasoning as
// TestARevokeRacingARotationNeverDeadlocksOrLeavesACredentialLive,
// oauth_lockorder_integration_test.go).
func TestTwoConsentsRacingOneHumansSeatNeverLeaveTwoActiveGrants(t *testing.T) {
	e := setupRevocationEnv(t, "oauth-supersede-race")

	const attempts = 12
	for attempt := range attempts {
		clientID := e.registerRaceClient(t, attempt)

		first, second, err := e.raceTwoConsents(e.admin, e.admin, clientID)
		if err != nil {
			t.Fatalf("attempt %d: a consent failed on the interleaving: %v", attempt, err)
		}

		if active := e.liveGrantsFor(t, clientID, e.admin); active != 1 {
			t.Fatalf("attempt %d: %d live grants on one human's seat, want 1 (first=%s second=%s)",
				attempt, active, first, second)
		}
	}
}

// TestTwoHumansRacingOneRegistrationEachKeepTheirOwnSeat is the same race
// across the seat boundary rather than inside it. It is the assertion that
// would have failed while supersede was keyed on client_id alone: the two
// consents serialize on nothing in common, and neither may cost the other its
// connection.
func TestTwoHumansRacingOneRegistrationEachKeepTheirOwnSeat(t *testing.T) {
	e := setupRevocationEnv(t, "oauth-supersede-race-cross-human")

	const attempts = 12
	for attempt := range attempts {
		clientID := e.registerRaceClient(t, attempt)

		adminGrant, memberGrant, err := e.raceTwoConsents(e.admin, e.member, clientID)
		if err != nil {
			t.Fatalf("attempt %d: a consent failed on the interleaving: %v", attempt, err)
		}

		var active int
		if err := e.owner.QueryRow(context.Background(),
			`SELECT count(*) FROM oauth_grant WHERE client_id = $1 AND revoked_at IS NULL`,
			clientID).Scan(&active); err != nil {
			t.Fatalf("attempt %d: counting live grants: %v", attempt, err)
		}
		if active != 2 {
			t.Fatalf("attempt %d: %d live grants for two humans on one registration, want 2 (admin=%s member=%s)",
				attempt, active, adminGrant, memberGrant)
		}
	}
}

// TestIssueGrantRefusesAClientDisabledUnderTheLock proves lockClientRegistration
// (oauth_grant.go) carries liveClientPredicate: a client disabled after the
// caller already validated it (consumeAuthCode does, in the same transaction,
// before issueGrant runs) must not still mint a grant because the lock query
// only checked client_id existed.
func TestIssueGrantRefusesAClientDisabledUnderTheLock(t *testing.T) {
	e := setupRevocationEnv(t, "oauth-supersede-disabled-client")

	clientID := "client-" + ids.NewV7().String()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO oauth_client (client_id, client_name, redirect_uris, disabled_at)
		VALUES ($1, 'disabled client', ARRAY['https://client.example/cb'], now())`, clientID); err != nil {
		t.Fatalf("registering the disabled client: %v", err)
	}

	_, _, err := e.issueGrantTx(e.admin, clientID)
	if !errors.Is(err, errCodeSpent) {
		t.Fatalf("issuing a grant for a disabled client: got %v, want errCodeSpent", err)
	}
}

// grantLive answers whether one grant row is still a connection, which is the
// only question every assertion in this file asks of the store.
func (e *revocationEnv) grantLive(t *testing.T, grantID ids.UUID) bool {
	t.Helper()
	var revokedAt *string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT revoked_at::text FROM oauth_grant WHERE id = $1`, grantID).Scan(&revokedAt); err != nil {
		t.Fatalf("reading grant %s: %v", grantID, err)
	}
	return revokedAt == nil
}

// liveGrantsFor counts one HUMAN's live connections on a registration — the
// seat the invariant bounds, as opposed to the registration-wide count, which
// is now expected to be as large as the number of humans using the client.
func (e *revocationEnv) liveGrantsFor(t *testing.T, clientID string, consenter Identity) int {
	t.Helper()
	var active int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM oauth_grant
		  WHERE client_id = $1 AND user_id = $2 AND revoked_at IS NULL`,
		clientID, consenter.UserID).Scan(&active); err != nil {
		t.Fatalf("counting live grants: %v", err)
	}
	return active
}

// registerRaceClient mints a fresh DCR registration per attempt. The full id,
// not a prefix: consecutive v7 ids share their leading bytes within a
// millisecond, and every attempt in this suite registers its own client.
func (e *revocationEnv) registerRaceClient(t *testing.T, attempt int) string {
	t.Helper()
	clientID := "client-" + ids.NewV7().String()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO oauth_client (client_id, client_name, redirect_uris)
		VALUES ($1, 'race client', ARRAY['https://client.example/cb'])`, clientID); err != nil {
		t.Fatalf("attempt %d: registering the client: %v", attempt, err)
	}
	return clientID
}

// raceTwoConsents commits two consents for one registration at once, as the
// two consenters given — the same human twice to race a seat from the inside,
// two humans to race across the boundary between seats.
func (e *revocationEnv) raceTwoConsents(first, second Identity, clientID string) (ids.UUID, ids.UUID, error) {
	var (
		wg                      sync.WaitGroup
		start                   = make(chan struct{})
		firstErr, secondErr     error
		firstGrant, secondGrant ids.UUID
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		firstGrant, _, firstErr = e.issueGrantTx(first, clientID)
	}()
	go func() {
		defer wg.Done()
		<-start
		secondGrant, _, secondErr = e.issueGrantTx(second, clientID)
	}()
	// Both goroutines block on start until they are scheduled and waiting, so
	// closing it releases them together instead of letting whichever the
	// runtime scheduled first get a head start on the other — the gap a missing
	// serialization point needs to slip through undetected.
	close(start)
	wg.Wait()

	return firstGrant, secondGrant, errors.Join(firstErr, secondErr)
}

// issueGrantTx runs issueGrant in its own transaction for the given consenter,
// the shape a real consent commits in — the one body both connectOAuthWithClient
// (which needs the fixture's t.Fatalf on failure) and the racing goroutines
// above (which need the error back to report the attempt) build on.
func (e *revocationEnv) issueGrantTx(consenter Identity, clientID string) (grantID ids.UUID, refresh string, err error) {
	ctx := e.wsCtx(consenter)
	err = e.svc.db.Tx(ctx, func(tx pgx.Tx) error {
		var txErr error
		grantID, refresh, txErr = issueGrant(ctx, tx, issueGrantInput{
			WorkspaceID: consenter.WorkspaceID, UserID: consenter.UserID, ClientID: clientID,
			Scopes: []string{"read"}, RefreshAllowed: true,
		})
		return txErr
	})
	return grantID, refresh, err
}

// connectOAuthWithClient is connectOAuthWith without minting a NEW client_id:
// the point of these tests is two different humans authorizing the SAME
// registration, so the client row already exists from the first call.
func (e *revocationEnv) connectOAuthWithClient(t *testing.T, consenter Identity, clientID string) connectFixture {
	t.Helper()
	out := connectFixture{clientID: clientID}
	var err error
	out.grantID, out.refresh, err = e.issueGrantTx(consenter, clientID)
	if err != nil {
		t.Fatalf("issuing the second grant: %v", err)
	}
	return out
}
