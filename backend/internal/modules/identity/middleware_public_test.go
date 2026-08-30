// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAssistantProfileIsExplicitlyPublic(t *testing.T) {
	if !isPublicRequest(httptest.NewRequest(http.MethodGet, "/v1/assistant/profile", nil)) {
		t.Fatal("assistant profile must pass the session gate for the login presence")
	}
	if isPublicRequest(httptest.NewRequest(http.MethodPost, "/v1/assistant/profile", nil)) {
		t.Fatal("assistant profile must expose GET anonymously, not every method on its path")
	}
}

// The consent flow's one asymmetry is METHOD-shaped, and the two methods carry
// opposite consequences: the GET must reach its handler without a session (to
// send that human somewhere they can get one), while the POST is the decision
// that lends their authority. Pinned here because matching the PATH instead
// would hand that decision to whoever can reach the URL.
func TestOnlyTheAuthorizeGetSurvivesWithoutASession(t *testing.T) {
	if !isConsentEntry(httptest.NewRequest(http.MethodGet, "/oauth/authorize", nil)) {
		t.Fatal("the authorize GET must reach its handler without a session, or a human who is not signed in has nowhere to be sent")
	}
	if isConsentEntry(httptest.NewRequest(http.MethodPost, "/oauth/authorize", nil)) {
		t.Fatal("the consent POST must demand a session: it lends the signed-in human's own authority")
	}
	// Nor is it a public path: a session the browser DOES carry still binds the
	// human the consent screen then belongs to.
	if isPublicRequest(httptest.NewRequest(http.MethodGet, "/oauth/authorize", nil)) {
		t.Fatal("the authorize GET must not skip authentication — it needs the human when there is one")
	}
}

func TestIsOIDCLoginRequest(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/v1/auth/oidc/google/start", true},
		{"/v1/auth/oidc/google/callback", true},
		{"/v1/auth/oidc/google/callback/extra", false},
		{"/v1/auth/oidc//start", false},
		{"/v1/auth/oidc/start", false},
		{"/v1/auth/login", false},
	}
	for _, c := range cases {
		if got := isOIDCLoginRequest(c.path); got != c.want {
			t.Errorf("isOIDCLoginRequest(%q) = %v, want %v", c.path, got, c.want)
		}
	}
	if !isPublicRequest(httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/google/start", nil)) {
		t.Fatal("the OIDC start route must be reachable without a session")
	}
	if isPublicRequest(httptest.NewRequest(http.MethodPost, "/v1/auth/oidc/google/start", nil)) {
		t.Fatal("the OIDC routes are GET-only in the contract; POST must not bypass the session gate")
	}
}
