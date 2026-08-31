// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The Google-app surface tells an operator which callback URLs to register on
// their OAuth client. Those URLs have one job — to be byte-identical to what
// Google receives — so what is DISPLAYED and what is SENT are held to each
// other here, in both directions. A mismatch is a redirect_uri_mismatch at the
// consent screen, which names nothing an operator can act on.

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// advertised returns the URL published for one purpose, and whether it is
// published at all.
func advertised(uris []crmcontracts.GoogleAppRedirectUri, purpose crmcontracts.GoogleAppRedirectUriPurpose) (string, bool) {
	for _, u := range uris {
		if u.Purpose == purpose {
			return u.Url, true
		}
	}
	return "", false
}

func TestTheAdvertisedSignInRedirectIsTheOneTheFlowSends(t *testing.T) {
	var s Server
	WithGoogleSignIn(GoogleSignInConfig{
		ClientID: "cid", ClientSecret: "secret", StateKey: "0123456789012345678901234567890123",
		RedirectBase: "https://api.example.com", PostLoginURL: "/", FailureURL: "/#/login?oidc=failed",
	})(&s, nil)

	shown, ok := advertised(s.redirectURIs, crmcontracts.SignIn)
	if !ok {
		t.Fatal("a deployment that composed sign-in advertises no sign-in redirect URI, so an operator is told to register nothing")
	}
	// The authorization request carries the redirect_uri the exchange will
	// repeat, so the consent redirect is where "what is sent" can be read.
	rec := httptest.NewRecorder()
	s.StartOidcSignIn(rec, httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/google/start", nil), "google")
	location := rec.Header().Get("Location")
	if location == "" {
		t.Fatal("the start route sent no redirect, so there is nothing to compare the advertised URI against")
	}
	consent, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parsing the authorization request: %v", err)
	}
	// The decoded parameter, compared EXACTLY. A substring check would pass on a
	// URL that merely contains the advertised one, and Google matches the
	// registered value byte for byte.
	sent := consent.Query().Get("redirect_uri")
	if sent != shown {
		t.Errorf("the sign-in URI advertised to the operator is %q, but the authorization request sends %q.\n"+
			"Registering the advertised value would fail redirect_uri_mismatch.", shown, sent)
	}
}

// The case an operator is actually in when they need this: they are CREATING
// the OAuth client, so no client id exists yet. Withholding the URI until one
// does would hide it exactly when it is needed, and leave them guessing the one
// string Google matches byte for byte.
func TestTheSignInRedirectIsAdvertisedBeforeTheAppIsConfigured(t *testing.T) {
	var s Server
	WithGoogleSignIn(GoogleSignInConfig{RedirectBase: "https://api.example.com"})(&s, nil)

	shown, ok := advertised(s.redirectURIs, crmcontracts.SignIn)
	if !ok {
		t.Fatal("an operator with no Google app yet is told no sign-in URI to register, which is the one they need to create it")
	}
	// And it is the SAME string the composed flow will send once they finish.
	var configured Server
	WithGoogleSignIn(GoogleSignInConfig{
		ClientID: "cid", ClientSecret: "secret", StateKey: "0123456789012345678901234567890123",
		RedirectBase: "https://api.example.com", PostLoginURL: "/", FailureURL: "/#/login?oidc=failed",
	})(&configured, nil)
	willSend, _ := advertised(configured.redirectURIs, crmcontracts.SignIn)
	if shown != willSend {
		t.Errorf("the URI advertised before configuration is %q but after it is %q; an operator would register the wrong one", shown, willSend)
	}
}

// A deployment that knows no origin of its own advertises nothing, rather than a
// URL built on an empty base.
func TestNoSignInRedirectIsAdvertisedWithoutABase(t *testing.T) {
	var s Server
	WithGoogleSignIn(GoogleSignInConfig{ClientID: "cid"})(&s, nil)
	if _, ok := advertised(s.redirectURIs, crmcontracts.SignIn); ok {
		t.Error("a deployment with no redirect base advertises a URI built on nothing")
	}
}

// The connector's URI is published on the same terms and for the same reason:
// an operator registers BOTH while creating the OAuth client, and registering
// only one fails at the consent screen with an error that does not say which.
//
// It is not derivable from the sign-in URI. This one prefers the API's own
// origin and falls back to the SPA's, while sign-in rides a base that already
// carries /v1 — on a split deployment they do not even share a host.
func TestTheConnectorRedirectIsAdvertisedAndIsNotTheSignInOne(t *testing.T) {
	var s Server
	WithGmailCapture(GmailConfig{
		PublicBaseURL: "https://app.example.com",
		APIBaseURL:    "https://api.example.com",
	}, CaptureConfig{})(&s, nil)

	shown, ok := advertised(s.redirectURIs, crmcontracts.MailboxConnect)
	if !ok {
		t.Fatal("an operator is told no mailbox-connect URI to register, so half the OAuth client stays unconfigured")
	}
	// The value the connect flow itself builds, from the same two bases.
	want := connectorCallbackURL("https://api.example.com", "https://app.example.com", "google")
	if shown != want {
		t.Errorf("advertised connector URI %q, but the connect flow sends %q", shown, want)
	}
	if signIn, _ := advertised(s.redirectURIs, crmcontracts.SignIn); signIn == shown {
		t.Error("the two purposes advertise the same URL, so one of them is wrong on a split deployment")
	}
}
