// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// Federated sign-in resolution and linking, proven over a real migrated
// Postgres: subject-first resolution, email fallback for a first link, the
// no-account-creation refusal, the subject re-link (email recycling) case,
// and the full HTTP round trip through OidcSignInCallback. ssologin_test.go
// covers the pure-Go handler edge cases (unconfigured provider, missing
// cookie) with stubs; this file is only for what genuinely needs a database.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity/internal/password"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedSSOEnv bootstraps a fresh installation (this suite's own workspace, per
// setupIdentityDB's per-test reset) with one active, ACTIVATED, unlinked
// app_user — a password_hash is set because that is what actually
// distinguishes an activated member from an unredeemed invite (both are
// `status = 'active'`; see resolveFederatedUser's doc comment). Tests for the
// un-activated case seed their own row without one, deliberately.
func seedSSOEnv(t *testing.T, slug string) (svc *Service, ownerConn *pgx.Conn, userID ids.UserID, email string) {
	t.Helper()
	ownerConn, pool := setupIdentityDB(t)
	ctx := context.Background()
	slug += "-" + ids.NewV7().String()[24:]

	var wsID ids.WorkspaceID
	err := database.WithInfraTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		wsID, err = createInstallation(ctx, tx, InstallationBootstrap{
			OrganizationName: slug,
			AdminEmail:       "admin@" + slug + ".test", AdminName: "Admin",
			AdminPassword: bootstrapPassword,
		}, originConfigured, nil, &[]string{})
		return err
	})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	svc = NewServiceFor(database.BindTo(pool, wsID))

	hash, err := password.Hash(bootstrapPassword)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	userID = ids.New[ids.UserKind]()
	email = "carol@" + slug + ".test"
	if _, err := ownerConn.Exec(ctx,
		`INSERT INTO app_user (id, email, password_hash, display_name) VALUES ($1, $2, $3, 'Carol')`,
		userID, email, hash); err != nil {
		t.Fatal(err)
	}
	return svc, ownerConn, userID, email
}

func linkFederatedIdentityRow(t *testing.T, conn *pgx.Conn, userID ids.UserID, subject, email string) {
	t.Helper()
	if _, err := conn.Exec(context.Background(),
		`INSERT INTO federated_identity (user_id, provider, subject, email_at_link) VALUES ($1, 'google', $2, $3)`,
		userID, subject, email); err != nil {
		t.Fatal(err)
	}
}

func TestResolveFederatedUserPrefersLinkedSubjectOverEmail(t *testing.T) {
	svc, conn, userID, email := seedSSOEnv(t, "sso-subject-first")
	linkFederatedIdentityRow(t, conn, userID, "sub-1", email)

	var resolved ids.UserID
	var isFirstLink bool
	err := svc.db.Tx(context.Background(), func(tx pgx.Tx) error {
		var resolveErr error
		resolved, isFirstLink, resolveErr = svc.resolveFederatedUser(context.Background(), tx, "google", "sub-1", email)
		return resolveErr
	})
	if err != nil {
		t.Fatalf("resolveFederatedUser: %v", err)
	}
	if resolved != userID {
		t.Fatalf("resolved = %v, want %v", resolved, userID)
	}
	if isFirstLink {
		t.Fatal("expected isFirstLink=false for an already-linked subject")
	}
}

func TestResolveFederatedUserFallsBackToEmailOnFirstLink(t *testing.T) {
	svc, _, userID, email := seedSSOEnv(t, "sso-email-fallback")

	var resolved ids.UserID
	var isFirstLink bool
	err := svc.db.Tx(context.Background(), func(tx pgx.Tx) error {
		var resolveErr error
		resolved, isFirstLink, resolveErr = svc.resolveFederatedUser(context.Background(), tx, "google", "sub-new", email)
		return resolveErr
	})
	if err != nil {
		t.Fatalf("resolveFederatedUser: %v", err)
	}
	if resolved != userID {
		t.Fatalf("resolved = %v, want %v", resolved, userID)
	}
	if !isFirstLink {
		t.Fatal("expected isFirstLink=true for an unlinked subject resolved by email")
	}
}

func TestResolveFederatedUserRefusesUnknownEmail(t *testing.T) {
	svc, _, _, _ := seedSSOEnv(t, "sso-unknown-email")

	err := svc.db.Tx(context.Background(), func(tx pgx.Tx) error {
		_, _, resolveErr := svc.resolveFederatedUser(context.Background(), tx, "google", "sub-x", "nobody@example.com")
		return resolveErr
	})
	if !errors.Is(err, ErrFederatedSignInRefused) {
		t.Fatalf("err = %v, want ErrFederatedSignInRefused for an email with no live app_user", err)
	}
}

// TestResolveFederatedUserRefusesADeactivatedLinkedSubject holds the
// resolveFederatedUser doc comment's own promise: an already-linked
// (provider, subject) must be refused exactly like an unrecognized password
// login once the user behind it is no longer live, not resolved straight
// from federated_identity with no LiveMemberSQL check at all.
func TestResolveFederatedUserRefusesADeactivatedLinkedSubject(t *testing.T) {
	svc, conn, userID, email := seedSSOEnv(t, "sso-deactivated-linked")
	linkFederatedIdentityRow(t, conn, userID, "sub-1", email)
	if _, err := conn.Exec(context.Background(),
		`UPDATE app_user SET status = 'deactivated' WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}

	err := svc.db.Tx(context.Background(), func(tx pgx.Tx) error {
		_, _, resolveErr := svc.resolveFederatedUser(context.Background(), tx, "google", "sub-1", email)
		return resolveErr
	})
	if !errors.Is(err, ErrFederatedSignInRefused) {
		t.Fatalf("err = %v, want ErrFederatedSignInRefused for a linked subject whose user is no longer live", err)
	}
}

// TestResolveFederatedUserRefusesAnUnactivatedInvite is the HIGH-severity
// finding the security review surfaced: an invited-never-activated member is
// written `status = 'active'` at invite time (there is no `invited` status —
// A97 specified one, ADR-0061 Amendment 1 dropped it as never built) and
// carries a NULL password_hash until the invite is redeemed. Without the
// password_hash check, that row's email is reachable by federated sign-in
// forever — no token, no expiry, unlike the invite email itself — the moment
// anyone controls that address on the IdP.
func TestResolveFederatedUserRefusesAnUnactivatedInvite(t *testing.T) {
	svc, conn, _, _ := seedSSOEnv(t, "sso-unactivated-invite")
	invitedEmail := "invited-never-activated@example.com"
	if _, err := conn.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name, status) VALUES ($1, $2, 'Invited', 'active')`,
		ids.New[ids.UserKind](), invitedEmail); err != nil {
		t.Fatal(err)
	}

	err := svc.db.Tx(context.Background(), func(tx pgx.Tx) error {
		_, _, resolveErr := svc.resolveFederatedUser(context.Background(), tx, "google", "sub-invite", invitedEmail)
		return resolveErr
	})
	if !errors.Is(err, ErrFederatedSignInRefused) {
		t.Fatalf("err = %v, want ErrFederatedSignInRefused for an active, un-activated (NULL password_hash) invite", err)
	}
}

// TestResolveFederatedUserRefusesTheAgentSeat holds the same refusal for the
// agent seat (installation.go's seedAgentSeat): it too is `status = 'active'`
// with no password_hash, and is not an authority a federated sign-in may
// stand up an interactive session for.
func TestResolveFederatedUserRefusesTheAgentSeat(t *testing.T) {
	svc, conn, _, _ := seedSSOEnv(t, "sso-agent-seat")
	agentEmail := "agent@sso-agent-seat.gradion.local"
	if _, err := conn.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name, is_agent, seat_type, status)
		 VALUES ($1, $2, 'Margince Agent', true, 'full', 'active')`,
		ids.New[ids.UserKind](), agentEmail); err != nil {
		t.Fatal(err)
	}

	err := svc.db.Tx(context.Background(), func(tx pgx.Tx) error {
		_, _, resolveErr := svc.resolveFederatedUser(context.Background(), tx, "google", "sub-agent", agentEmail)
		return resolveErr
	})
	if !errors.Is(err, ErrFederatedSignInRefused) {
		t.Fatalf("err = %v, want ErrFederatedSignInRefused for the agent seat", err)
	}
}

// TestLinkFederatedIdentityTransfersAStaleSubjectFromANonLiveUser proves
// linkFederatedIdentity's other unique constraint holds: the same
// (provider, subject) can already be linked to a user resolveFederatedUser
// found not live/activated, which fell through to resolve a DIFFERENT live
// user by email (the recycling case this whole flow exists for). The insert
// must transfer the subject rather than tripping
// federated_identity_provider_subject_key underneath the caller.
func TestLinkFederatedIdentityTransfersAStaleSubjectFromANonLiveUser(t *testing.T) {
	svc, conn, liveUserID, liveEmail := seedSSOEnv(t, "sso-stale-subject-transfer")

	staleUserID := ids.New[ids.UserKind]()
	if _, err := conn.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name, status) VALUES ($1, $2, 'Departed', 'deactivated')`,
		staleUserID, "departed@example.com"); err != nil {
		t.Fatal(err)
	}
	linkFederatedIdentityRow(t, conn, staleUserID, "sub-recycled", "departed@example.com")

	// The live user's email now verifies for the SAME Google subject
	// (Google-side email reassignment) — resolveFederatedUser falls through
	// the dead link to resolve liveUserID by email, and linking must not
	// fail on the stale row's hold over (provider, subject).
	if _, err := svc.LoginViaFederatedIdentity(context.Background(), "google", "sub-recycled", liveEmail); err != nil {
		t.Fatalf("LoginViaFederatedIdentity: %v", err)
	}

	var owner ids.UserID
	if err := conn.QueryRow(context.Background(),
		`SELECT user_id FROM federated_identity WHERE provider = 'google' AND subject = 'sub-recycled'`).Scan(&owner); err != nil {
		t.Fatalf("reading federated_identity: %v", err)
	}
	if owner != liveUserID {
		t.Fatalf("owner = %v, want the subject transferred to the live user %v", owner, liveUserID)
	}
	var staleRowCount int
	if err := conn.QueryRow(context.Background(),
		`SELECT count(*) FROM federated_identity WHERE user_id = $1`, staleUserID).Scan(&staleRowCount); err != nil {
		t.Fatal(err)
	}
	if staleRowCount != 0 {
		t.Fatalf("the stale user's federated_identity row should have been retired, found %d", staleRowCount)
	}
}

func TestLoginViaFederatedIdentityRelinksOnSubjectChange(t *testing.T) {
	svc, conn, userID, email := seedSSOEnv(t, "sso-relink")
	linkFederatedIdentityRow(t, conn, userID, "sub-old", email)

	// A different Google account now presents the same verified, previously
	// linked email — the recycling case linkFederatedIdentity's ON CONFLICT
	// clause exists for.
	if _, err := svc.LoginViaFederatedIdentity(context.Background(), "google", "sub-new-after-recycle", email); err != nil {
		t.Fatalf("LoginViaFederatedIdentity: %v", err)
	}

	var subject string
	if err := conn.QueryRow(context.Background(),
		`SELECT subject FROM federated_identity WHERE user_id = $1 AND provider = 'google'`, userID).Scan(&subject); err != nil {
		t.Fatalf("reading federated_identity: %v", err)
	}
	if subject != "sub-new-after-recycle" {
		t.Fatalf("subject = %q, want the new subject to have overwritten the old link", subject)
	}
}

// fixedVerifier/fixedExchanger/fixedStateSigner (ssologin_test.go) satisfy
// the three injected interfaces with values fixed at construction — the full
// callback round trip needs a real database (LoginViaFederatedIdentity) but
// no real Google, so only those three collaborators are faked.

func TestOIDCSignInFullRoundTrip(t *testing.T) {
	svc, _, userID, email := seedSSOEnv(t, "sso-full-round-trip")

	h := Handlers{svc: svc}.WithOIDCProviders(
		map[string]OIDCProviderConfig{"google": {
			Key: "google", ClientID: "cid",
			AuthURL: "https://accounts.google.com/o/oauth2/v2/auth",
		}},
		map[string]OIDCVerifier{"google": fixedVerifier{email: email, sub: "sub-carol", emailVerified: true}},
		map[string]OIDCExchanger{"google": fixedExchanger{idToken: "unused-by-fixedVerifier"}},
		fixedStateSigner{provider: "google", nonce: "n1", codeVerifier: "v1"},
		OIDCRoutes{RedirectBase: "https://app.example.com", PostLoginURL: "/", FailureURL: "/#/login?oidc=failed"},
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/google/callback?code=c&state=n1", nil)
	req.AddCookie(&http.Cookie{Name: oidcLoginCookie, Value: "irrelevant-fixedStateSigner-ignores-it"})
	rec := httptest.NewRecorder()

	h.OidcSignInCallback(rec, req, "google", callbackParams("c", "n1"))

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/" {
		t.Fatalf("status=%d location=%q", rec.Code, rec.Header().Get("Location"))
	}
	var sessionCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookieName {
			sessionCookie = c
		}
	}
	// The success path always calls clearLoginStateCookie before
	// setSessionCookie, so the response carries the expired oidc_login_state
	// cookie either way — checking cookie COUNT alone would pass even if
	// setSessionCookie itself were broken or removed.
	if sessionCookie == nil || sessionCookie.Value == "" {
		t.Fatal("expected a non-empty session cookie to be set")
	}
	var count int
	if err := svc.db.Tx(context.Background(), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM federated_identity WHERE user_id = $1 AND provider = 'google'`, userID).Scan(&count)
	}); err != nil {
		t.Fatalf("counting federated_identity rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("federated_identity rows = %d, want 1", count)
	}
}

func TestOIDCSignInFullRoundTripRefusesUnknownEmail(t *testing.T) {
	svc, _, _, _ := seedSSOEnv(t, "sso-round-trip-unknown-email")

	h := Handlers{svc: svc}.WithOIDCProviders(
		map[string]OIDCProviderConfig{"google": {
			Key: "google", ClientID: "cid",
			AuthURL: "https://accounts.google.com/o/oauth2/v2/auth",
		}},
		map[string]OIDCVerifier{"google": fixedVerifier{email: "nobody@example.com", sub: "sub-nobody", emailVerified: true}},
		map[string]OIDCExchanger{"google": fixedExchanger{idToken: "unused-by-fixedVerifier"}},
		fixedStateSigner{provider: "google", nonce: "n1", codeVerifier: "v1"},
		OIDCRoutes{RedirectBase: "https://app.example.com", PostLoginURL: "/", FailureURL: "/#/login?oidc=failed"},
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/google/callback?code=c&state=n1", nil)
	req.AddCookie(&http.Cookie{Name: oidcLoginCookie, Value: "irrelevant-fixedStateSigner-ignores-it"})
	rec := httptest.NewRecorder()

	h.OidcSignInCallback(rec, req, "google", callbackParams("c", "n1"))

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/#/login?oidc=failed" {
		t.Fatalf("status=%d location=%q, want 302 to the failure URL", rec.Code, rec.Header().Get("Location"))
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookieName {
			t.Fatal("a refused sign-in must never set the session cookie")
		}
	}
}
