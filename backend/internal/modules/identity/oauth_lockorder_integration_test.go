// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// The lock order across the two paths that touch a connection's rows: a human
// revoking a connection at the instant the connector renews it must QUEUE on
// the grant row, not deadlock against a refresh row the other side grabbed
// first. Both paths take oauth_grant before oauth_refresh_token; this suite
// runs them against each other over a real Postgres, because a lock order is
// only ever a property of real transactions.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// connectFixture is one consented connection: a registered client, the grant
// beneath it, and the refresh token the connector holds.
type connectFixture struct {
	clientID string
	grantID  ids.UUID
	refresh  string
}

// connectOAuth mints a connection the admin consented to.
func (e *revocationEnv) connectOAuth(t *testing.T) connectFixture {
	t.Helper()
	return e.connectOAuthFor(t, e.admin)
}

// connectOAuthFor mints a connection through the module's own issuance path, so
// the fixture is the same shape the code exchange commits. The consenting human
// is a parameter because what ends a connection includes what ends THEIR
// access, and an admin is the one human deactivation refuses to touch.
func (e *revocationEnv) connectOAuthFor(t *testing.T, consenter Identity) connectFixture {
	t.Helper()
	// The lock-order suite tests revoke/rotation races, not lend races.
	return e.connectOAuthWith(t, consenter, "lock order")
}

// connectOAuthWith builds a connection through the module's own issuance
// path, taking the client name as a parameter so a caller can register more
// than one client in the same test. Named apart from connectOAuth/
// connectOAuthFor above (both pre-existing, narrower convenience wrappers over
// this one) rather than reusing either name, which would collide.
func (e *revocationEnv) connectOAuthWith(
	t *testing.T, consenter Identity, clientName string,
) connectFixture {
	t.Helper()
	out := connectFixture{clientID: e.registerOAuthClient(t, clientName)}
	ctx := e.wsCtx(consenter)
	if err := e.svc.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out.grantID, out.refresh, err = issueGrant(ctx, tx, consentInput(consenter, out.clientID))
		return err
	}); err != nil {
		t.Fatalf("issuing the grant: %v", err)
	}
	return out
}

// registerOAuthClient registers a client with no connection under it yet and
// returns its client_id.
func (e *revocationEnv) registerOAuthClient(t *testing.T, clientName string) string {
	t.Helper()
	// The full id, not a prefix: consecutive v7 ids share their leading bytes
	// within a millisecond, and every attempt in this suite registers its own
	// client.
	clientID := "client-" + ids.NewV7().String()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO oauth_client (client_id, client_name, redirect_uris)
		VALUES ($1, $2, ARRAY['https://client.example/cb'])`,
		clientID, clientName); err != nil {
		t.Fatalf("registering the client: %v", err)
	}
	return clientID
}

func consentInput(consenter Identity, clientID string) issueGrantInput {
	return issueGrantInput{
		WorkspaceID: consenter.WorkspaceID, UserID: consenter.UserID, ClientID: clientID,
		Scopes: []string{"read"}, RefreshAllowed: true,
	}
}

// TestARevokeRacingARotationNeverDeadlocksOrLeavesACredentialLive fires the
// two paths at one grant simultaneously. Whoever wins, two things must hold:
// neither side may fail with a database error — a lock-order inversion shows
// up here as a deadlock abort, which the caller can only answer with a 500 on
// an operation the lock exists to serialize — and a revoked connection must
// have nothing live left behind it, however the two interleaved.
//
// The race is run repeatedly because the interleaving is the scheduler's
// choice, not ours: one pass proves little, and a pass that never deadlocks
// across many attempts is the strongest deterministic statement available. A
// true deadlock cannot be forced from outside without observing the other
// transaction's locks, so this suite proves the absence, not the presence.
func TestARevokeRacingARotationNeverDeadlocksOrLeavesACredentialLive(t *testing.T) {
	e := setupRevocationEnv(t, "oauth-lock-order")

	const attempts = 12
	for attempt := range attempts {
		fixture := e.connectOAuth(t)

		var (
			wg          sync.WaitGroup
			rotateErr   error
			revokeErr   error
			rotationCtx = e.wsCtx(e.admin)
			revokeCtx   = e.wsCtx(e.admin)
		)
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _, rotateErr = e.svc.rotateRefreshToken(rotationCtx, refreshRequest{
				Token: fixture.refresh, ClientID: fixture.clientID,
			})
		}()
		go func() {
			defer wg.Done()
			revokeErr = e.svc.db.Tx(revokeCtx, func(tx pgx.Tx) error {
				return revokeGrantTx(revokeCtx, tx, fixture.grantID, "the human ended the connection")
			})
		}()
		wg.Wait()

		// The rotation either won the grant lock and renewed, or queued behind
		// the revoke and found the connection dead. Any other error is the
		// database refusing the interleaving.
		if rotateErr != nil && !errors.Is(rotateErr, errRefreshRejected) {
			t.Fatalf("attempt %d: rotation failed on the interleaving, not on the rule: %v", attempt, rotateErr)
		}
		if revokeErr != nil {
			t.Fatalf("attempt %d: revoke failed on the interleaving: %v", attempt, revokeErr)
		}

		// Whatever the order, a revoked connection holds no live credential:
		// if the rotation won, the cascade caught the passport and the
		// successor it had just minted.
		e.assertNothingLiveUnder(t, fixture.grantID, fmt.Sprintf("attempt %d", attempt))
	}
}

// The human's own kill switch is the second path into the cascade, and it
// reaches it with a passport id rather than a grant id — so it is the path that
// could take a passport lock before the grant's and invert the order. It races
// the same way, and it must also refuse to become a no-op: a rotation that
// replaces the named credential a moment earlier must not leave the connection
// serving calls under the successor.
func TestAPassportRevokeRacingARotationNeverDeadlocksOrSparesTheConnection(t *testing.T) {
	e := setupRevocationEnv(t, "passport-revoke-lock-order")

	const attempts = 12
	for attempt := range attempts {
		fixture := e.connectOAuth(t)
		passportID := e.mintUnderGrant(t, fixture.grantID)

		var (
			wg          sync.WaitGroup
			rotateErr   error
			revokeErr   error
			rotationCtx = e.wsCtx(e.admin)
			revokeCtx   = e.wsCtx(e.admin)
		)
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _, rotateErr = e.svc.rotateRefreshToken(rotationCtx, refreshRequest{
				Token: fixture.refresh, ClientID: fixture.clientID,
			})
		}()
		go func() {
			defer wg.Done()
			revokeErr = e.svc.RevokePassport(revokeCtx, e.admin, passportID)
		}()
		wg.Wait()

		if rotateErr != nil && !errors.Is(rotateErr, errRefreshRejected) {
			t.Fatalf("attempt %d: rotation failed on the interleaving, not on the rule: %v", attempt, rotateErr)
		}
		if revokeErr != nil {
			t.Fatalf("attempt %d: revoking the passport failed on the interleaving: %v", attempt, revokeErr)
		}
		// Whichever won, the human's revoke is the last word: nothing under the
		// grant is usable, including a passport the rotation minted in between.
		e.assertNothingLiveUnder(t, fixture.grantID, fmt.Sprintf("attempt %d", attempt))
	}
}

// The same property the race above can only sample, proven deterministically:
// the human aims at the passport their Settings screen listed, a rotation
// replaced it in the meantime, and the connection must still die. Treating the
// already-dead row as "nothing left to do" is what leaves the connector working
// after a human deliberately cut it off — the successor credential is live and
// the grant beneath it is untouched.
func TestRevokingAPassportARotationAlreadyReplacedStillEndsTheConnection(t *testing.T) {
	e := setupRevocationEnv(t, "passport-revoke-after-rotation")
	fixture := e.connectOAuth(t)
	replaced := e.mintUnderGrant(t, fixture.grantID)

	if _, _, err := e.svc.rotateRefreshToken(e.wsCtx(e.admin), refreshRequest{
		Token: fixture.refresh, ClientID: fixture.clientID,
	}); err != nil {
		t.Fatalf("rotating the connection: %v", err)
	}

	if err := e.svc.RevokePassport(e.wsCtx(e.admin), e.admin, replaced); err != nil {
		t.Fatalf("revoking the replaced passport: %v", err)
	}
	e.assertNothingLiveUnder(t, fixture.grantID, "after revoking a replaced passport")
}

// An admin deleting a client while a consent to it commits queues on the client
// row, the lock both take first. The connection the consent made then dies too.
func TestDeletingAClientWhileAConsentCommitsEndsTheConnectionItMade(t *testing.T) {
	e := setupRevocationEnv(t, "client-delete-behind-consent")
	clientID := e.registerOAuthClient(t, "deleted mid-consent")
	rowID := e.oauthClientRowID(t, clientID)
	var grantID ids.UUID
	consent := e.holdOpen(t, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		grantID, _, err = issueGrant(ctx, tx, consentInput(e.admin, clientID))
		return err
	})

	deleted := make(chan error, 1)
	returned := make(chan struct{})
	go func() {
		defer close(returned)
		deleted <- e.svc.DeleteAgentClient(e.wsCtx(e.admin), e.admin, rowID)
	}()
	e.awaitQueuedBehind(t, consent.pid, returned, "the delete")
	consent.release()

	if err := <-consent.done; err != nil {
		t.Fatalf("the consent failed on the interleaving: %v", err)
	}
	if err := <-deleted; err != nil {
		t.Fatalf("the delete failed on the interleaving: %v", err)
	}
	e.assertNothingLiveUnder(t, grantID, "after a delete queued behind the consent")
}

// The other order: the delete holds the client row. The consent queued behind
// it finds the client gone and is refused the way a spent code is.
func TestAConsentQueuedBehindAClientDeleteIsRefused(t *testing.T) {
	e := setupRevocationEnv(t, "consent-behind-client-delete")
	clientID := e.registerOAuthClient(t, "deleted before consent")
	rowID := e.oauthClientRowID(t, clientID)
	deletion := e.holdOpen(t, func(ctx context.Context, tx pgx.Tx) error {
		return revokeClientTx(ctx, tx, rowID)
	})

	consented := make(chan error, 1)
	returned := make(chan struct{})
	ctx := e.wsCtx(e.admin)
	go func() {
		defer close(returned)
		consented <- e.svc.db.Tx(ctx, func(tx pgx.Tx) error {
			_, _, err := issueGrant(ctx, tx, consentInput(e.admin, clientID))
			return err
		})
	}()
	e.awaitQueuedBehind(t, deletion.pid, returned, "the consent")
	deletion.release()

	if err := <-deletion.done; err != nil {
		t.Fatalf("the delete failed on the interleaving: %v", err)
	}
	if err := <-consented; !errors.Is(err, errCodeSpent) {
		t.Fatalf("a consent queued behind the delete → %v, want the spent-code refusal", err)
	}
	var grants int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM oauth_grant WHERE client_id = $1`, clientID).Scan(&grants); err != nil {
		t.Fatalf("counting the grants: %v", err)
	}
	if grants != 0 {
		t.Fatalf("a deleted client gained %d grant(s)", grants)
	}
}

// heldTx is a transaction kept open, with every lock it took, until release.
// pid is its backend, so a probe can ask who queues behind it.
type heldTx struct {
	pid     int32
	release func()
	done    <-chan error
}

// holdOpen runs work as the admin and holds its transaction open. Release is
// also a cleanup, so a test that fails mid-race never strands the locks.
func (e *revocationEnv) holdOpen(t *testing.T, work func(context.Context, pgx.Tx) error) heldTx {
	t.Helper()
	ctx := e.wsCtx(e.admin)
	held := make(chan int32, 1)
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- e.svc.db.Tx(ctx, func(tx pgx.Tx) error {
			if err := work(ctx, tx); err != nil {
				return err
			}
			var pid int32
			if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				return err
			}
			held <- pid
			<-release
			return nil
		})
	}()
	var once sync.Once
	releaseOnce := func() { once.Do(func() { close(release) }) }
	t.Cleanup(releaseOnce)
	select {
	case pid := <-held:
		return heldTx{pid: pid, release: releaseOnce, done: done}
	case err := <-done:
		t.Fatalf("the held transaction ended before it held anything: %v", err)
		return heldTx{}
	}
}

// awaitQueuedBehind returns once some backend waits on a lock pid holds. A
// deadlock never gets here: Postgres aborts one side, and the racer returns.
func (e *revocationEnv) awaitQueuedBehind(t *testing.T, pid int32, racerReturned <-chan struct{}, racer string) {
	t.Helper()
	testdb.WaitForContention(t, racerReturned,
		racer+" finished while the other side held the client row: it never queued on the lock both take first",
		racer+" never queued behind the client row the other side held",
		func(ctx context.Context) (bool, error) {
			if _, err := e.owner.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
				return false, err
			}
			var queued bool
			err := e.owner.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity a
				  WHERE $1 = ANY (pg_blocking_pids(a.pid)))`, pid).Scan(&queued)
			return queued, err
		})
}

func (e *revocationEnv) oauthClientRowID(t *testing.T, clientID string) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT id FROM oauth_client WHERE client_id = $1`, clientID).Scan(&id); err != nil {
		t.Fatalf("reading the client row: %v", err)
	}
	return id
}

// mintUnderGrant issues the credential a code exchange would have minted
// beneath the admin's grant, so the cases above have a passport to revoke.
func (e *revocationEnv) mintUnderGrant(t *testing.T, grantID ids.UUID) ids.PassportID {
	t.Helper()
	return e.mintUnderGrantFor(t, grantID, e.admin)
}

// mintUnderGrantFor is mintUnderGrant for a connection some other human
// consented to.
func (e *revocationEnv) mintUnderGrantFor(t *testing.T, grantID ids.UUID, consenter Identity) ids.PassportID {
	t.Helper()
	ctx := e.wsCtx(consenter)
	var issued IssuedPassport
	if err := e.svc.db.Tx(ctx, func(tx pgx.Tx) error {
		label := oauthPassportLabel("lock order")
		var err error
		issued, err = mintPassport(ctx, tx, consenter,
			IssuePassportInput{Label: &label, Scopes: []string{"read"}}, &grantID)
		return err
	}); err != nil {
		t.Fatalf("minting the passport under the grant: %v", err)
	}
	return issued.ID
}

// assertNothingLiveUnder is the end state a revoked connection must always
// reach: the grant dead, no spendable refresh token, no usable passport.
func (e *revocationEnv) assertNothingLiveUnder(t *testing.T, grantID ids.UUID, when string) {
	t.Helper()
	ctx := context.Background()
	var revoked bool
	if err := e.owner.QueryRow(ctx,
		`SELECT revoked_at IS NOT NULL FROM oauth_grant WHERE id = $1`, grantID).Scan(&revoked); err != nil {
		t.Fatalf("%s: reading the grant: %v", when, err)
	}
	if !revoked {
		t.Fatalf("%s: the grant survived its own revocation", when)
	}
	var spendable, usable int
	if err := e.owner.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM oauth_refresh_token WHERE grant_id = $1 AND consumed_at IS NULL),
		       (SELECT count(*) FROM passport WHERE oauth_grant_id = $1 AND revoked_at IS NULL)`,
		grantID).Scan(&spendable, &usable); err != nil {
		t.Fatalf("%s: reading the credentials under the grant: %v", when, err)
	}
	if spendable != 0 || usable != 0 {
		t.Fatalf("%s: revoked connection left %d spendable refresh token(s) and %d usable passport(s)",
			when, spendable, usable)
	}
}
