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
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// OIDCProviderConfig is one configured external identity provider, injected
// by compose. "google" is the only key today; the shape is generic so a
// second provider is a config entry, not a refactor. It carries only what
// ssologin.go itself reads (ClientID for the authorization request, AuthURL
// to redirect to) — the client secret and token endpoint belong to the
// OIDCExchanger compose injects separately (googleTokenExchanger), and this
// struct is not a second place for them to live.
type OIDCProviderConfig struct {
	Key      string
	Label    string
	ClientID string
	AuthURL  string
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
	oidcVerifierLen     = 64 // 64 bytes -> an 86-char base64url verifier, inside RFC 7636's 43-128 character range.
	oidcLoginCookie     = "oidc_login_state"
	oidcLoginCookiePath = "/v1/auth/oidc"
	// oidcLoggedErrorMaxLen bounds how much of Google's `error` query value
	// this route ever writes to system_log. It is a public, unauthenticated
	// callback the browser's address bar reaches directly, so the value is
	// caller-controlled — the per-IP limiter bounds request RATE, not the
	// SIZE of what one request can persist. Standard OAuth error codes
	// (access_denied, invalid_scope, ...) are a handful of characters; this
	// is generous headroom for those, not an attempt to keep a longer value.
	oidcLoggedErrorMaxLen = 64
)

// truncateForLog caps s to at most n bytes for a log/audit field a caller
// controls — never used on a value already bounded by its own contract (a
// UUID, an enum). It backs off to the nearest preceding UTF-8 rune boundary
// rather than cutting at a raw byte index: this value is written to
// system_log as jsonb text, and Postgres REJECTS invalid UTF-8 outright
// (error 22021) — a mid-rune cut would fail that write silently (this is a
// best-effort audit trail) and lose the very refusal record the split
// exists to keep, precisely for the attacker-controlled multi-byte value
// this function exists to bound.
func truncateForLog(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

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
	// The budget is spent BEFORE the policy read, because that read goes to the
	// database: checking it first would let an unauthenticated caller drive
	// unlimited settings queries without ever consuming their allowance.
	//
	// Ordering it this way discloses nothing. Exhaustion answers the same 429
	// whether or not the provider exists, so a caller learns only that they were
	// noisy — the 404 below stays the single answer for "no such provider" and
	// "the admin turned it off" alike.
	if !h.oidcPerIP.Allow(httpserver.ClientIP(r)) {
		httperr.Write(w, r, apperrors.ErrBudgetExceeded)
		return
	}
	// A failed policy read is NOT the 404: reporting an outage as absence would
	// send an operator to debug a configuration that is fine.
	enabled, policyErr := h.oidcProviderEnabled(r.Context(), provider)
	if policyErr != nil {
		httperr.Write(w, r, policyErr)
		return
	}
	if !enabled {
		httperr.Write(w, r, apperrors.ErrNotFound)
		return
	}
	nonce, err := randomTokenOfLength(oidcNonceBytes)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	verifier, err := randomTokenOfLength(oidcVerifierLen)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	challenge := sha256.Sum256([]byte(verifier))
	token := h.stateSigner.Sign(provider, nonce, verifier, oidcStateTTL)
	setLoginStateCookie(w, token, oidcStateTTL)

	q := url.Values{
		"client_id":             {cfg.ClientID},
		"redirect_uri":          {h.callbackURI(provider)},
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
	ctx := r.Context()
	// fail redirects to the neutral marker and records the cause SERVER-SIDE
	// only — never echoed to the client, same posture as errOIDCRejected's
	// own doc comment. A state-verify failure, an exchange failure, a
	// rejected ID token, an unrecognized email, and a genuine database
	// failure are all rendered as the same redirect, but an operator
	// debugging a broken Google app — or watching for a brute-force against
	// this route — needs the reason, exactly like recordFailedLogin
	// (lockout.go) exists to give one for the password path. Takes ctx
	// explicitly (the request's, captured once above) rather than closing
	// over r: the detached write inside logOidcFailure derives its own
	// timeout from whichever ctx is handed to it, so this must be the same
	// one every other step below already uses.
	fail := func(ctx context.Context, reason string, cause error) {
		h.logOidcFailure(ctx, provider, reason, cause)
		http.Redirect(w, r, h.oidcRoutes.FailureURL, http.StatusFound)
	}

	if _, ok := h.oidcProviders[provider]; !ok {
		httperr.Write(w, r, apperrors.ErrNotFound)
		return
	}
	if !h.oidcPerIP.Allow(httpserver.ClientIP(r)) {
		httperr.Write(w, r, apperrors.ErrBudgetExceeded)
		return
	}
	// Checked on the CALLBACK too, not only at the start, and after the budget
	// for the same reason it is there. A provider disabled while a browser was
	// away at the consent screen must not be able to complete: filtering the
	// button row alone would leave this route minting sessions for every flow
	// already in flight.
	enabled, policyErr := h.oidcProviderEnabled(ctx, provider)
	if policyErr != nil {
		fail(ctx, "provider policy", policyErr)
		return
	}
	if !enabled {
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

	// The cookie is read and VERIFIED before it is cleared, and before the
	// `error` check below — both on purpose. This callback is a public GET,
	// the cookie is SameSite=Lax (so it rides a cross-site top-level
	// navigation), and `state`/`error` are both attacker-suppliable. Clearing
	// on mere presence (the shape an earlier version of this code, and of
	// the `error` check, both had) would let a forged cross-site link
	// consume and cancel a victim's real, unrelated, still-in-progress
	// sign-in merely by being clicked — their subsequent legitimate return
	// from Google would then fail "no state cookie" for a flow they never
	// abandoned. Only a request whose cookie decrypts to a genuinely
	// matching provider+nonce ever reaches the clear.
	cookie, err := r.Cookie(oidcLoginCookie)
	if err != nil {
		fail(ctx, "no state cookie", nil)
		return
	}
	stProvider, stNonce, codeVerifier, err := h.stateSigner.Verify(cookie.Value)
	if err != nil || stProvider != provider || stNonce != state {
		fail(ctx, "state verification", err)
		return
	}
	clearLoginStateCookie(w) // one-shot: consumed now that the state has genuinely matched

	// Google sends `error` instead of `code` when the user denies consent —
	// e.g. access_denied. Checked here, now that the state above has proven
	// this is genuinely the browser's own pending flow.
	if params.Error != nil && *params.Error != "" {
		fail(ctx, "provider error: "+truncateForLog(*params.Error, oidcLoggedErrorMaxLen), nil)
		return
	}

	email, sub, reason, err := h.exchangeAndVerify(ctx, provider, code, codeVerifier)
	if reason != "" {
		fail(ctx, reason, err)
		return
	}

	token, err := h.svc.LoginViaFederatedIdentity(ctx, provider, sub, email)
	if err != nil {
		fail(ctx, "resolve/link account", err)
		return
	}
	setSessionCookie(w, token)
	http.Redirect(w, r, h.oidcRoutes.PostLoginURL, http.StatusFound)
}

// exchangeAndVerify redeems the authorization code and validates the ID
// token it returns, split out of OidcSignInCallback so that function's own
// branching stays over the state/cookie plumbing rather than growing to
// cover the token round trip too. A non-empty reason means refuse; email/sub
// are meaningful only when reason is empty.
func (h Handlers) exchangeAndVerify(ctx context.Context, provider, code, codeVerifier string) (email, sub, reason string, err error) {
	idToken, err := h.oidcExchangers[provider].Exchange(ctx, code, codeVerifier, h.callbackURI(provider))
	if err != nil {
		return "", "", "token exchange", err
	}
	email, sub, emailVerified, err := h.oidcVerifiers[provider].Verify(ctx, idToken)
	if err != nil {
		return "", "", "id token verification", err
	}
	if !emailVerified {
		return "", "", "email not verified", nil
	}
	// email/sub are both required to reach here (they identify who signed
	// in and are what LoginViaFederatedIdentity resolves/links against) —
	// the verifier contract does not itself guarantee either is non-empty,
	// so an unchecked blank value would resolve/link a blank identity
	// rather than being refused here.
	if email == "" || sub == "" {
		return "", "", "missing email or subject claim", nil
	}
	return email, sub, "", nil
}

// logOidcFailure writes one system_log row for a refused/failed OIDC
// sign-in, mirroring recordFailedLogin's shape (lockout.go): a login that
// mutates no record belongs in the operational ledger, not audit_log, and an
// invisible failure trail is exactly what a brute-force or a broken IdP
// config both need to be caught by. Detached like recordFailedLogin — the
// browser has already been told "refused" by the time this runs regardless
// of outcome, so the write is the installation's record, not the client's to
// cancel. Best-effort: a failure to write the trail must not turn an
// already-refused sign-in into a 500 on top of it, so the write's own error
// goes to the server log only.
func (h Handlers) logOidcFailure(ctx context.Context, provider, reason string, cause error) {
	slog.WarnContext(ctx, "oidc sign-in refused", "provider", provider, "reason", reason, "err", cause)
	if h.svc == nil {
		return
	}
	writeCtx, cancel := detachedForFailure(ctx)
	defer cancel()
	if err := h.svc.db.Tx(writeCtx, func(tx pgx.Tx) error {
		_, err := tx.Exec(writeCtx,
			`INSERT INTO system_log (actor_type, actor_id, action, detail)
			 VALUES ('human', 'human:unauthenticated', 'login',
			         jsonb_build_object('outcome', 'failed'::text, 'provider', $1::text, 'reason', $2::text))`,
			provider, reason)
		return err
	}); err != nil {
		slog.ErrorContext(ctx, "recording a refused oidc sign-in", "err", err)
	}
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
// UNLINKED subject falls back to email, through LiveMemberSQL AND
// password_hash IS NOT NULL — the same pair checkCredentials (lockout.go)
// and reset.go's forgot-password lookup already require.
//
// BOTH halves are load-bearing, and neither is redundant now that `invited` is
// a status the tree actually writes. LiveMemberSQL excludes an unredeemed
// invitation by status. password_hash IS NOT NULL excludes the AGENT SEAT,
// which installation.go seeds `active` with a NULL hash and which must never
// be a thing that signs in — and it also holds the line if a future path ever
// activates an account without setting a credential.
//
// Skipping either would let an unredeemed invitation, or the agent seat,
// become reachable by anyone who controls that address on the IdP, forever —
// no token, no expiry, unlike the invitation mail itself. That is the whole
// reason redemption, and not this branch, is what activates an account.
func (s *Service) resolveFederatedUser(ctx context.Context, tx pgx.Tx, provider, subject, email string) (userID ids.UserID, firstLink bool, err error) {
	var linkedUser ids.UserID
	err = tx.QueryRow(ctx,
		`SELECT fi.user_id FROM federated_identity fi
		 JOIN app_user u ON u.id = fi.user_id
		 WHERE fi.provider = $1 AND fi.subject = $2 AND `+LiveMemberSQL("u")+`
		 AND u.password_hash IS NOT NULL AND NOT u.is_agent`,
		provider, subject).Scan(&linkedUser)
	switch {
	case err == nil:
		return linkedUser, false, nil
	case errors.Is(err, pgx.ErrNoRows):
		// Either genuinely unlinked, or linked to a user who is no longer
		// live/activated — both fall through to email resolution below, and
		// both must land on the SAME refusal as an unrecognized password
		// login: a suspended, archived, un-activated, or agent account's
		// still-valid link must not read as a successful sign-in just
		// because the row exists.
	default:
		return ids.UserID{}, false, fmt.Errorf("identity: resolve federated identity: %w", err)
	}

	var byEmail ids.UserID
	err = tx.QueryRow(ctx,
		`SELECT id FROM app_user WHERE `+LiveMemberSQL("")+`
		 AND password_hash IS NOT NULL AND NOT is_agent AND lower(email) = lower($1)`,
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
// The table carries TWO unique constraints — (user_id, provider) and
// (provider, subject) — and this function answers to both. ON CONFLICT
// (user_id, provider) means a SUBJECT CHANGE for an existing link updates
// rather than errors: the email-recycling case, where a different Google
// account now presents the same verified email an old link used. That case
// is not silently indistinguishable from a normal login: the caller passes a
// distinct audit detail for it (see LoginViaFederatedIdentity).
//
// The DELETE before it answers the other constraint: the same (provider,
// subject) can already be linked to a DIFFERENT user_id — the row
// resolveFederatedUser found not live, not activated, or an agent seat, and
// fell through past to resolve a different (live, activated) user by email.
// That stale row still holds the (provider, subject) unique slot, so the
// insert below would hit federated_identity_provider_subject_key instead of
// the (user_id, provider) conflict target it's written for. Retiring it here
// transfers the subject to the user the caller already decided to sign in,
// rather than refusing a login on an internal constraint the caller never
// sees.
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

	if _, err := tx.Exec(ctx,
		`DELETE FROM federated_identity WHERE provider = $1 AND subject = $2 AND user_id <> $3`,
		provider, subject, userID); err != nil {
		return false, fmt.Errorf("identity: retire stale federated identity: %w", err)
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
