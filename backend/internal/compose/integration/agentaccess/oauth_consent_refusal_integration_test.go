// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package agentaccess

// What a HUMAN sees when the consent flow refuses them. The screen is a native
// form in the SPA, so a JSON refusal replaces it with a body of text on a page
// that has no navigation: the human's flow ends there, with no way back and
// nothing to click. Every refusal a human can cause therefore comes back to the
// screen with a marker naming it.
//
// Every one of them is TERMINAL — consentScreenRefusal is the only shape left.
// The lend flow this replaced had a RECOVERABLE case too (a passport revoked in
// another tab, where only the human's choice was wrong and the pending
// authorization survived it); ticked scopes cannot go stale between render and
// submit the way a passport row could, so that second shape has nothing left to
// test.
//
// The refusals a CLIENT causes are unchanged: deny still answers the client's
// own redirect_uri (TestDenyRedirectsToTheClientWithAccessDenied, below), and
// a cross-site POST is still refused outright rather than redirected
// (TestOAuthConsentGateBlocksSilentAuthorization).

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// consentScreenHandback reads the fragment a refused consent POST answered with
// and asserts what EVERY refusal owes the screen: the screen's own route, and the
// authorization request back with it — without which the human is at a form with
// nothing to re-post.
func consentScreenHandback(t *testing.T, status int, location string) url.Values {
	t.Helper()
	if status != http.StatusFound {
		t.Fatalf("refused consent POST → %d, want 302 back to the consent screen", status)
	}
	params := consentFragment(t, location)
	for _, param := range []string{"client_id", "redirect_uri", "scope", "code_challenge", "state"} {
		if params.Get(param) == "" {
			t.Fatalf("the refusal dropped %s, which the screen must re-post: %q", param, location)
		}
	}
	return params
}

// consentScreenRefusal reads the marker off a TERMINAL refusal: one the human
// cannot act on, where the screen states that the connection has to be started
// again from the client.
//
// The nonce must be absent, checked twice — as a parameter and as a substring of
// the whole Location. A re-entry carrying it could only be refused again for the
// same reason, so a screen offered it would present an action that never works.
func consentScreenRefusal(t *testing.T, status int, location, spentNonce string) string {
	t.Helper()
	params := consentScreenHandback(t, status, location)
	if got := params.Get("consent"); got != "" {
		t.Fatalf("a terminal refusal hands the screen a nonce %q; nothing it could submit would be accepted", got)
	}
	if spentNonce == "" {
		t.Fatal("the caller passed no armed nonce, so the leak check below would pass vacuously")
	}
	if strings.Contains(location, spentNonce) {
		t.Fatalf("the spent nonce leaked into the refusal %q", location)
	}
	return params.Get("error")
}

// A nonce the browser can no longer prove is what a human who left the screen
// open past the cookie's five minutes produces — the ordinary case, not an
// attack. The screen gets them back, and re-entry arms a fresh nonce.
func TestAStaleConsentNonceComesBackToTheScreen(t *testing.T) {
	o := setupOAuth(t)
	form := o.armConsent(t, nil)
	armed := form.Get("consent")
	form.Set("consent", "not-the-nonce-this-browser-was-given")

	status, location, _ := o.postConsent(t, form)

	if got := consentScreenRefusal(t, status, location, armed); got != "stale_consent" {
		t.Fatalf("error = %q, want stale_consent: %q", got, location)
	}
	// The nonce check runs before anything durable can exist, and still does.
	assertOwnerCount(t, o, 0, `SELECT count(*) FROM oauth_authorization_code`)
	assertOwnerCount(t, o, 0,
		`SELECT count(*) FROM audit_log WHERE entity_type = 'oauth_authorization_code'`)
}

// The other end of that cookie's life. A refusal leaves the armed pair alone, so
// something has to retire it — and that something is the DECISION: the approve
// that minted a code and the deny that answered the client each clear the cookie
// on their way out, which is what stops one armed authorization from being
// submitted over and over for the rest of its five minutes.
func TestACommittedConsentCannotBeSubmittedTwice(t *testing.T) {
	o := setupOAuth(t)
	for name, decision := range map[string]struct {
		decide func(*testing.T, *oauthEnv, url.Values)
		answer string
	}{
		"an approve that minted a code": {
			decide: func(t *testing.T, _ *oauthEnv, form url.Values) {
				t.Helper()
				form.Set("scopes", "read")
			},
			answer: "code=",
		},
		"a deny that answered the client": {
			decide: func(t *testing.T, _ *oauthEnv, form url.Values) {
				t.Helper()
				form.Set("deny", "1")
			},
			answer: "error=access_denied",
		},
	} {
		t.Run(name, func(t *testing.T) {
			form := o.armConsent(t, url.Values{"scope": {"read"}})
			armed := form.Get("consent")
			decision.decide(t, o, form)

			status, location, body := o.postConsent(t, form)
			if status != http.StatusFound || !strings.Contains(location, decision.answer) {
				t.Fatalf("the decision → %d %q %s, want a 302 carrying %s", status, location, body, decision.answer)
			}

			// The very same form again, nonce and all. The browser no longer holds
			// the cookie half of the pair, so this is refused — and refused
			// TERMINALLY, because there is nothing left to decide.
			status, location, _ = o.postConsent(t, form)
			if got := consentScreenRefusal(t, status, location, armed); got != "stale_consent" {
				t.Fatalf("re-submitting a committed consent → %q, want stale_consent: %q", got, location)
			}
		})
	}
	// One code per approve, however many times the form was submitted: the deny
	// subtest above mints none at all, so this is the approve's single row.
	assertOwnerCount(t, o, 1, `SELECT count(*) FROM oauth_authorization_code`)
}

// A POST whose authorization request no longer validates: the parameters are
// mutated AFTER the nonce was armed, so the double-submit check passes and
// validateAuthorize is the thing that refuses. The human reads the refusal on
// the screen; the specific OAuth code is a client developer's vocabulary and
// stays on the GET, where a client developer looks.
func TestAConsentPostThatFailsValidationComesBackToTheScreen(t *testing.T) {
	o := setupOAuth(t)
	form := o.armConsent(t, nil)
	armed := form.Get("consent")
	form.Set("scopes", "read")
	// The OAuth 2.1 downgrade validateAuthorize refuses: S256 is mandatory.
	form.Set("code_challenge_method", "plain")

	status, location, _ := o.postConsent(t, form)

	if got := consentScreenRefusal(t, status, location, armed); got != "invalid_request" {
		t.Fatalf("error = %q, want invalid_request: %q", got, location)
	}
	assertOwnerCount(t, o, 0, `SELECT count(*) FROM oauth_authorization_code`)
}

// A consent POST that ticks nothing at all cannot come from a screen this
// server rendered — every checkbox defaults ticked — so it is a hand-built
// form or a bug, and the refusal is TERMINAL rather than a second chance.
// mintConsentedAuthorizationCode refuses with apperrors.ErrInvalidArgument,
// and oauth.go's consent handler turns that into the same invalid_request the
// screen shows for any other malformed POST.
func TestAConsentPostWithNoScopesTickedComesBackToTheScreen(t *testing.T) {
	o := setupOAuth(t)
	form := o.armConsent(t, nil)
	armed := form.Get("consent")
	form.Set("scopes", "")

	status, location, _ := o.postConsent(t, form)

	if got := consentScreenRefusal(t, status, location, armed); got != "invalid_request" {
		t.Fatalf("error = %q, want invalid_request: %q", got, location)
	}
	// Nothing durable exists for a scope list this server would never have
	// rendered: no code to redeem later, no audit row naming a grant that
	// never happened.
	assertOwnerCount(t, o, 0, `SELECT count(*) FROM oauth_authorization_code`)
	assertOwnerCount(t, o, 0,
		`SELECT count(*) FROM audit_log WHERE entity_type = 'oauth_authorization_code'`)
}

// A scope outside the closed read|draft|write|send|enrich vocabulary is the
// same case as no scopes at all: this server rendered five checkboxes over a
// fixed vocabulary, so a POST naming a sixth did not come from that screen.
func TestAConsentPostWithAnOutOfVocabularyScopeComesBackToTheScreen(t *testing.T) {
	o := setupOAuth(t)
	form := o.armConsent(t, nil)
	armed := form.Get("consent")
	form.Set("scopes", "read admin")

	status, location, _ := o.postConsent(t, form)

	if got := consentScreenRefusal(t, status, location, armed); got != "invalid_request" {
		t.Fatalf("error = %q, want invalid_request: %q", got, location)
	}
	assertOwnerCount(t, o, 0, `SELECT count(*) FROM oauth_authorization_code`)
	assertOwnerCount(t, o, 0,
		`SELECT count(*) FROM audit_log WHERE entity_type = 'oauth_authorization_code'`)
}

// A human arriving from `claude mcp add` in a fresh browser has no session, and
// this endpoint serves no HTML to ask for one. The SPA does: AuthGate renders
// login in place at the route it was handed, so the answer is the screen —
// carrying the request and nothing this endpoint has not yet done. After signing
// in the screen re-enters /oauth/authorize, which is where a nonce comes from.
func TestAnUnauthenticatedAuthorizeGetRoutesTheHumanToSignIn(t *testing.T) {
	o := setupOAuth(t)

	req, err := http.NewRequest(http.MethodGet,
		o.TS.URL+"/oauth/authorize?"+o.authorizeQuery(nil).Encode(), nil)
	if err != nil {
		t.Fatal(err)
	}
	anonymous := o.sessionlessClient()
	anonymous.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := anonymous.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer apptest.CloseBody(t, resp)

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("unauthenticated authorize GET → %d, want 302 to the consent screen (a JSON 401 is a dead end)", resp.StatusCode)
	}
	location := resp.Header.Get("Location")
	params := consentFragment(t, location)
	// No nonce: there is no human yet to bind one to, and the screen must
	// re-enter this endpoint after sign-in to obtain one.
	if got := params.Get("consent"); got != "" {
		t.Fatalf("an unauthenticated GET armed a nonce %q", got)
	}
	if len(resp.Cookies()) != 0 {
		t.Fatalf("an unauthenticated GET set cookies %v, so it armed something before knowing who asked", resp.Cookies())
	}
	// The request survives the detour, or signing in loses the connection the
	// human was trying to make.
	if params.Get("client_id") != o.clientID {
		t.Fatalf("client_id = %q, want %q — the request must survive the trip through login", params.Get("client_id"), o.clientID)
	}
	// The target is the SPA document, which this api does not serve: nothing
	// behind this redirect redirects again on its own, so there is no loop.
	if strings.HasPrefix(location, "/oauth/") {
		t.Fatalf("Location = %q points back at the authorization server", location)
	}

	// Where the walk comes to rest: the screen re-enters with exactly the
	// parameters it was handed, now carrying a session, and THAT arms the nonce.
	// Driven from `params` rather than from a freshly built query, so the
	// re-entry is the one the SPA can actually make.
	status, reentry, body, cookies := o.authorizeNoFollow(t, params)
	if status != http.StatusFound {
		t.Fatalf("re-entry with a session → %d %s, want 302", status, body)
	}
	nonce := consentFragment(t, reentry).Get("consent")
	if nonce == "" {
		t.Fatalf("re-entry armed no nonce, so the POST could never satisfy the double-submit check: %q", reentry)
	}
	if got := cookieValue(t, cookies, consentCookieName); got != nonce {
		t.Fatalf("cookie %s = %q, want the fragment's fresh nonce %q", consentCookieName, got, nonce)
	}
}

// denyRaw is the human refusing. RFC 6749 §4.1.2.1 answers the CLIENT at its
// own redirect_uri, so the status and Location are the whole observable
// outcome — there is no code to hand back.
func (o *oauthEnv) denyRaw(t *testing.T, extra url.Values) (int, string) {
	t.Helper()
	form := o.armConsent(t, extra)
	form.Set("deny", "1")
	status, location, _ := o.postConsent(t, form)
	return status, location
}

// Deny is a first-class answer: the client is TOLD, per RFC 6749 §4.1.2.1,
// rather than left hanging on a closed tab. This holds regardless of how
// consent is decided — it predates the lend flow's passport_id and survives
// its replacement by ticked scopes unchanged, because a deny never reaches
// that decision at all.
func TestDenyRedirectsToTheClientWithAccessDenied(t *testing.T) {
	o := setupOAuth(t)

	status, location := o.denyRaw(t, url.Values{"scope": {"read"}})

	if status != http.StatusFound {
		t.Fatalf("deny → %d, want 302", status)
	}
	if !strings.HasPrefix(location, oauthRedirect) {
		t.Fatalf("Location = %q, want the client's redirect_uri", location)
	}
	if !strings.Contains(location, "error=access_denied") {
		t.Fatalf("Location = %q must carry error=access_denied", location)
	}
	// state is echoed or the client cannot correlate the refusal with its request.
	if !strings.Contains(location, "state=night-state") {
		t.Fatalf("Location = %q must echo state", location)
	}
	// A refusal is not a quiet approval: the redirect carries no code, and no
	// code row was written for one to be drawn from later. Nothing was granted,
	// so there is deliberately no lend to audit either.
	if strings.Contains(location, "code=") {
		t.Fatalf("Location = %q carries a code although the human refused", location)
	}
	assertOwnerCount(t, o, 0, `SELECT count(*) FROM oauth_authorization_code`)
	assertOwnerCount(t, o, 0,
		`SELECT count(*) FROM audit_log WHERE entity_type = 'oauth_authorization_code'`)
}
