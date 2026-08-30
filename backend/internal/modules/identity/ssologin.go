// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Google/OIDC sign-in for an already-invited member. This is identity acting
// as an OAuth CLIENT against a third-party IdP — the opposite direction from
// oauth.go, which is identity acting as an OAuth SERVER issuing Agent Seat
// Passports. Same module, opposite role; kept in its own file so the two are
// never confused.
//
// No account creation happens here (root design decision): an email with no
// live app_user is a neutral failure, exactly like an unrecognized password
// login. See resolveFederatedUser.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// OIDCProviderConfig is one configured external identity provider, injected
// by compose. "google" is the only key today; the shape is generic so a
// second provider is a config entry, not a refactor.
type OIDCProviderConfig struct {
	Key          string
	Label        string
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
}

// OIDCVerifier is what ssologin needs from an ID-token verifier — defined
// here, not imported from compose, so identity never depends on compose (a
// module never imports a sibling; compose injects the edge instead).
type OIDCVerifier interface {
	Verify(ctx context.Context, idToken string) (email, sub string, emailVerified bool, err error)
}

// OIDCExchanger is what ssologin needs from the code exchange.
type OIDCExchanger interface {
	Exchange(ctx context.Context, code, codeVerifier, redirectURI string) (idToken string, err error)
}

// OIDCStateSigner is what ssologin needs from the signed-state mechanism —
// exported for the same reason as OIDCVerifier/OIDCExchanger. compose's
// loginStateSignerAdapter satisfies this; identity never sees compose's HMAC
// details.
type OIDCStateSigner interface {
	Sign(provider, nonce, codeVerifier string, ttl time.Duration) (token string)
	Verify(token string) (provider, nonce, codeVerifier string, err error)
}

const (
	oidcStateTTL        = 10 * time.Minute
	oidcNonceBytes      = 32
	oidcVerifierLen     = 64 // RFC 7636 recommends 43-128 chars; 64 is comfortably inside that.
	oidcLoginCookie     = "oidc_login_state"
	oidcLoginCookiePath = "/v1/auth/oidc"
)

// setLoginStateCookie/clearLoginStateCookie own the one cookie this flow
// needs, right beside setSessionCookie (handlers.go) which already does the
// same for crm_session — ordinary net/http, no compose dependency. Lax (not
// Strict) because it must ride the top-level redirect back from Google;
// HttpOnly because the PKCE verifier inside must never reach JS; one-shot,
// cleared once Callback consumes it.
func setLoginStateCookie(w http.ResponseWriter, token string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name: oidcLoginCookie, Value: token, Path: oidcLoginCookiePath,
		MaxAge: int(ttl.Seconds()), HttpOnly: true, Secure: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearLoginStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: oidcLoginCookie, Path: oidcLoginCookiePath, MaxAge: -1,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	})
}

func randomURLSafe(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("identity: read random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// StartOidcSignIn redirects to the provider's consent screen. Unconfigured
// or unknown provider is a 404 — an authentically absent flow. The provider
// parameter type is generated from crm.yaml's enumerated path parameter.
func (h Handlers) StartOidcSignIn(w http.ResponseWriter, r *http.Request, providerParam crmcontracts.StartOidcSignInParamsProvider) {
	provider := string(providerParam)
	cfg, ok := h.oidcProviders[provider]
	if !ok {
		httperr.Write(w, r, apperrors.ErrNotFound)
		return
	}
	nonce, err := randomURLSafe(oidcNonceBytes)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	verifier, err := randomURLSafe(oidcVerifierLen)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	challenge := sha256.Sum256([]byte(verifier))
	token := h.stateSigner.Sign(provider, nonce, verifier, oidcStateTTL)
	setLoginStateCookie(w, token, oidcStateTTL)

	redirectURI := h.oidcRedirectBase + "/auth/oidc/" + provider + "/callback"
	q := url.Values{
		"client_id":             {cfg.ClientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {oauthResponseTypeCode},
		"scope":                 {"openid email profile"},
		"state":                 {nonce},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"},
	}
	http.Redirect(w, r, cfg.AuthURL+"?"+q.Encode(), http.StatusFound)
}

// OidcSignInCallback verifies the round trip, resolves/links the account,
// and mints a session — or redirects to a neutral failure marker. Every
// error path below redirects rather than returning a JSON error: this route
// is reached by a full-page browser navigation, not an API caller.
func (h Handlers) OidcSignInCallback(w http.ResponseWriter, r *http.Request, providerParam crmcontracts.OidcSignInCallbackParamsProvider, params crmcontracts.OidcSignInCallbackParams) {
	provider := string(providerParam)
	fail := func() {
		http.Redirect(w, r, h.oidcFailureURL, http.StatusFound)
	}

	if _, ok := h.oidcProviders[provider]; !ok {
		httperr.Write(w, r, apperrors.ErrNotFound)
		return
	}

	var code, state string
	if params.Code != nil {
		code = *params.Code
	}
	if params.State != nil {
		state = *params.State
	}

	cookie, err := r.Cookie(oidcLoginCookie)
	if err != nil {
		fail()
		return
	}
	clearLoginStateCookie(w) // one-shot: consumed here whether verification below succeeds or not
	stProvider, stNonce, codeVerifier, err := h.stateSigner.Verify(cookie.Value)
	if err != nil || stProvider != provider || stNonce != state {
		fail()
		return
	}

	idToken, err := h.oidcExchangers[provider].Exchange(r.Context(), code, codeVerifier,
		h.oidcRedirectBase+"/auth/oidc/"+provider+"/callback")
	if err != nil {
		fail()
		return
	}
	email, sub, emailVerified, err := h.oidcVerifiers[provider].Verify(r.Context(), idToken)
	if err != nil || !emailVerified {
		fail()
		return
	}

	token, err := h.svc.LoginViaFederatedIdentity(r.Context(), provider, sub, email)
	if err != nil {
		fail()
		return
	}
	setSessionCookie(w, token)
	http.Redirect(w, r, h.oidcPostLoginURL, http.StatusFound)
}

// ErrFederatedSignInRefused is the one neutral failure ssologin.go ever
// returns to its callers — deliberately indistinguishable between "no such
// email", "account not live", and "email not verified", for the same
// no-enumeration reason /auth/login refuses to distinguish "no such email"
// from "wrong password".
var ErrFederatedSignInRefused = errors.New("identity: federated sign-in refused")

// resolveFederatedUser answers which app_user a verified (provider, subject,
// email) tuple belongs to, and whether this is the first time this provider
// has been linked to that user. It tries (provider, subject) FIRST: an
// already-linked identity resolves without touching email at all. Only an
// UNLINKED subject falls back to email, through LiveMemberSQL — the same
// "still works here" predicate the password path already uses
// (lockout.go), so an unknown, suspended, or archived account is refused
// exactly like an unrecognized password login.
func (s *Service) resolveFederatedUser(ctx context.Context, tx pgx.Tx, provider, subject, email string) (userID ids.UserID, firstLink bool, err error) {
	var linkedUser ids.UserID
	err = tx.QueryRow(ctx,
		`SELECT fi.user_id FROM federated_identity fi
		 JOIN app_user u ON u.id = fi.user_id
		 WHERE fi.provider = $1 AND fi.subject = $2 AND `+LiveMemberSQL("u"),
		provider, subject).Scan(&linkedUser)
	switch {
	case err == nil:
		return linkedUser, false, nil
	case errors.Is(err, pgx.ErrNoRows):
		// Either genuinely unlinked, or linked to a user who is no longer
		// live — both fall through to email resolution below, and both must
		// land on the SAME refusal as an unrecognized password login: a
		// suspended or archived account's still-valid link must not read as
		// a successful sign-in just because the row exists.
	default:
		return ids.UserID{}, false, fmt.Errorf("identity: resolve federated identity: %w", err)
	}

	var byEmail ids.UserID
	err = tx.QueryRow(ctx,
		`SELECT id FROM app_user WHERE `+LiveMemberSQL("")+` AND lower(email) = lower($1)`,
		email).Scan(&byEmail)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.UserID{}, false, ErrFederatedSignInRefused
	}
	if err != nil {
		return ids.UserID{}, false, fmt.Errorf("identity: resolve app_user by email: %w", err)
	}
	return byEmail, true, nil
}

// linkFederatedIdentity records the (provider, subject) -> user_id mapping.
// ON CONFLICT (user_id, provider) means a SUBJECT CHANGE for an existing
// link updates rather than errors — the email-recycling case, where a
// different Google account now presents the same verified email an old link
// used. That case is not silently indistinguishable from a normal login: the
// caller passes a distinct audit detail for it (see LoginViaFederatedIdentity).
func linkFederatedIdentity(ctx context.Context, tx pgx.Tx, userID ids.UserID, provider, subject, email string) (wasRelink bool, err error) {
	var existingSubject string
	scanErr := tx.QueryRow(ctx,
		`SELECT subject FROM federated_identity WHERE user_id = $1 AND provider = $2`,
		userID, provider).Scan(&existingSubject)
	switch {
	case scanErr == nil:
		wasRelink = existingSubject != subject
	case errors.Is(scanErr, pgx.ErrNoRows):
		wasRelink = false
	default:
		return false, fmt.Errorf("identity: read existing federated identity: %w", scanErr)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO federated_identity (user_id, provider, subject, email_at_link)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_id, provider)
		 DO UPDATE SET subject = EXCLUDED.subject, email_at_link = EXCLUDED.email_at_link`,
		userID, provider, subject, email)
	if err != nil {
		return false, fmt.Errorf("identity: link federated identity: %w", err)
	}
	return wasRelink, nil
}

// LoginViaFederatedIdentity resolves a verified (provider, subject, email)
// tuple to a session, mirroring Service.Login's shape: mint the token first,
// then one transaction that links/resolves, mints the session row, and
// audits — the same unexported session helpers Login already uses, no
// parallel implementation. Sessions carry no workspace column (ADR-0091 §8),
// so unlike Login this needs no bound installation context.
func (s *Service) LoginViaFederatedIdentity(ctx context.Context, provider, subject, email string) (string, error) {
	rawToken, tokenHash, err := mintSessionToken()
	if err != nil {
		return "", fmt.Errorf("identity: mint session token: %w", err)
	}

	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		userID, firstLink, resolveErr := s.resolveFederatedUser(ctx, tx, provider, subject, email)
		if resolveErr != nil {
			return resolveErr
		}
		wasRelink, linkErr := linkFederatedIdentity(ctx, tx, userID, provider, subject, email)
		if linkErr != nil {
			return linkErr
		}
		if insErr := insertSession(ctx, tx, userID, tokenHash); insErr != nil {
			return fmt.Errorf("identity: insert session: %w", insErr)
		}
		detail := fmt.Sprintf("oidc login: %s", provider)
		switch {
		case wasRelink:
			detail = fmt.Sprintf("oidc re-link: %s (subject changed)", provider)
		case firstLink:
			detail = fmt.Sprintf("oidc login: %s (first link)", provider)
		}
		return auditLogin(ctx, tx, userID, detail)
	})
	if err != nil {
		if errors.Is(err, ErrFederatedSignInRefused) {
			return "", ErrFederatedSignInRefused
		}
		return "", err
	}
	return rawToken, nil
}
