// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The refusals /oauth/authorize gives before any client row is read. The
// Handlers here carry no Service, so a request that got past the refusal would
// panic on the missing database rather than pass by accident.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const testConsentNonce = "the-nonce-the-screen-was-handed"

// authorizeForm is a request every check ahead of the registration settings
// admits.
func authorizeForm(clientID string) url.Values {
	return url.Values{
		oauthParamResponseType:        {oauthResponseTypeCode},
		oauthParamClientID:            {clientID},
		oauthParamRedirectURI:         {"https://client.example/cb"},
		oauthParamScope:               {"read"},
		oauthParamState:               {"night-state"},
		oauthParamCodeChallenge:       {strings.Repeat("c", 43)},
		oauthParamCodeChallengeMethod: {pkceMethodS256},
	}
}

func signedInRequest(method, target string, body *strings.Reader) *http.Request {
	r := httptest.NewRequest(method, target, body)
	return r.WithContext(withHumanPrincipal(context.Background(), Identity{}))
}

// consentPost is the screen's approval. Both halves of the nonce pair get it to
// validateAuthorize, and its ticked scope leaves no other refusal before a code.
func consentPost(form url.Values) *http.Request {
	form.Set(consentScreenParamNonce, testConsentNonce)
	form.Set("scopes", "read")
	r := signedInRequest(http.MethodPost, "/oauth/authorize", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: consentCookie, Value: testConsentNonce})
	return r
}

// screenRefusal is the reason a redirect to the consent screen carries, and
// fails the test if the answer was anything else.
func screenRefusal(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	location := rec.Header().Get("Location")
	_, query, found := strings.Cut(location, ConsentScreenPath+"?")
	if rec.Code != http.StatusFound || !found {
		t.Fatalf("got %d to %q, want a redirect to the consent screen", rec.Code, location)
	}
	params, err := url.ParseQuery(query)
	if err != nil {
		t.Fatalf("parsing the consent screen's parameters %q: %v", query, err)
	}
	if params.Get(consentScreenParamNonce) != "" {
		t.Errorf("a refusal hands the screen a nonce: %q", location)
	}
	return params.Get(consentScreenParamError)
}

var unreadableSwitches = switchesReturning(RegistrationMethods{}, errors.New("db down"))

// The client reads the GET's answer, so an outage is said to the client in its
// own vocabulary: try again later, not "you are refused".
func TestTheSignInStartIsUnavailableWhenTheSwitchCannotBeRead(t *testing.T) {
	h := Handlers{}.WithClientRegistration(allMethods, unreadableSwitches)
	rec := httptest.NewRecorder()
	h.oauthConsentRedirect(rec, signedInRequest(http.MethodGet,
		"/oauth/authorize?"+authorizeForm("some-client").Encode(), strings.NewReader("")))

	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), oauthErrTemporarilyUnavailable) {
		t.Errorf("authorize with an unreadable switch → %d %s, want 503 temporarily_unavailable", rec.Code, rec.Body)
	}
	if rec.Header().Get("Set-Cookie") != "" {
		t.Error("a refused sign-in start armed a consent nonce")
	}
}

// The human's browser reads the POST's answer. So the outage lands on the
// screen as a request it cannot complete, and nothing is minted.
func TestAConsentIsRefusedOnTheScreenWhenTheSwitchCannotBeRead(t *testing.T) {
	h := Handlers{}.WithClientRegistration(allMethods, unreadableSwitches)
	rec := httptest.NewRecorder()
	h.oauthAuthorize(rec, consentPost(authorizeForm("some-client")))

	if reason := screenRefusal(t, rec); reason != consentErrorInvalid {
		t.Errorf("consent with an unreadable switch → screen reason %q, want %q", reason, consentErrorInvalid)
	}
}

// A metadata-document client is refused before it is fetched or looked up. A
// human who approves one after its method was switched off is told so on screen.
func TestAConsentThroughASwitchedOffMethodIsSaidOnTheScreen(t *testing.T) {
	h := Handlers{}.WithClientRegistration(allMethods,
		switchesReturning(RegistrationMethods{Dynamic: true}, nil))
	rec := httptest.NewRecorder()
	h.oauthAuthorize(rec, consentPost(authorizeForm("https://client.example/oauth/metadata.json")))

	if reason := screenRefusal(t, rec); reason != consentErrorMethodOff {
		t.Errorf("consent through a switched-off method → screen reason %q, want %q", reason, consentErrorMethodOff)
	}
}
