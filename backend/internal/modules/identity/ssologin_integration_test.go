// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// Federated sign-in resolution and linking, proven over a real migrated
// Postgres: subject-first resolution, email fallback for a first link, the
// no-account-creation refusal, the subject re-link (email recycling) case,
// and the full HTTP round trip through OIDCSignInCallback. ssologin_test.go
// covers the pure-Go handler edge cases (unconfigured provider, missing
// cookie) with stubs; this file is only for what genuinely needs a database.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedSSOEnv bootstraps a fresh installation (this suite's own workspace, per
// setupIdentityDB's per-test reset) with one active, unlinked app_user.
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

	userID = ids.New[ids.UserKind]()
	email = "carol@" + slug + ".test"
	if _, err := ownerConn.Exec(ctx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Carol')`, userID, email); err != nil {
		t.Fatal(err)
	}
	return svc, ownerConn, userID, email
}

func linkFederatedIdentityRow(t *testing.T, conn *pgx.Conn, userID ids.UserID, provider, subject, email string) {
	t.Helper()
	if _, err := conn.Exec(context.Background(),
		`INSERT INTO federated_identity (user_id, provider, subject, email_at_link) VALUES ($1, $2, $3, $4)`,
		userID, provider, subject, email); err != nil {
		t.Fatal(err)
	}
}

func TestResolveFederatedUserPrefersLinkedSubjectOverEmail(t *testing.T) {
	svc, conn, userID, email := seedSSOEnv(t, "sso-subject-first")
	linkFederatedIdentityRow(t, conn, userID, "google", "sub-1", email)

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
	if err == nil {
		t.Fatal("expected a refusal for an email with no live app_user")
	}
}

func TestLoginViaFederatedIdentityRelinksOnSubjectChange(t *testing.T) {
	svc, conn, userID, email := seedSSOEnv(t, "sso-relink")
	linkFederatedIdentityRow(t, conn, userID, "google", "sub-old", email)

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
		map[string]OIDCProviderConfig{"google": {Key: "google", ClientID: "cid", ClientSecret: "secret",
			AuthURL: "https://accounts.google.com/o/oauth2/v2/auth", TokenURL: "https://oauth2.googleapis.com/token"}},
		map[string]OIDCVerifier{"google": fixedVerifier{email: email, sub: "sub-carol", emailVerified: true}},
		map[string]OIDCExchanger{"google": fixedExchanger{idToken: "unused-by-fixedVerifier"}},
		fixedStateSigner{provider: "google", nonce: "n1", codeVerifier: "v1"},
		"https://app.example.com", "/", "/#/login?oidc=failed",
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/google/callback?code=c&state=n1", nil)
	req.AddCookie(&http.Cookie{Name: oidcLoginCookie, Value: "irrelevant-fixedStateSigner-ignores-it"})
	rec := httptest.NewRecorder()

	h.OIDCSignInCallback(rec, req, "google")

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/" {
		t.Fatalf("status=%d location=%q", rec.Code, rec.Header().Get("Location"))
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("expected a session cookie to be set")
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
