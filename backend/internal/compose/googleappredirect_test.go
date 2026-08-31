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
	"strings"
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

// Every connector this Google app backs is advertised, on the path its own route
// is served under, and the three URLs are distinct.
//
// What this holds is the PATH TEMPLATE and the distinctness — not that the key
// matches the dispatch switch, which it cannot: both sides read the same
// constants, so they move together. Comparing an advertised URI against the
// builder that produced it would hold nothing at all, which is how an earlier
// shape of this test passed while every mailbox URI named a path no route
// answers.
func TestEveryGoogleBackedConnectorIsAdvertisedUnderItsOwnRouteKey(t *testing.T) {
	var s Server
	WithGmailCapture(GmailConfig{
		PublicBaseURL: "https://app.example.com",
		APIBaseURL:    "https://api.example.com",
	}, CaptureConfig{})(&s, nil)

	// The same ordered list the wiring reads, so a connector added there without
	// a URI shows up here rather than being silently unadvertised.
	for _, connector := range googleBackedConnectors {
		purpose, provider := connector.purpose, connector.provider
		shown, ok := advertised(s.redirectURIs, purpose)
		if !ok {
			t.Errorf("%s is not advertised, so that half of the OAuth client stays unregistered", purpose)
			continue
		}
		// The path ConnectConnector serves for this provider, spelled from the
		// same constant the dispatch switch reads.
		if want := "/v1/connectors/" + provider + "/callback"; !strings.HasSuffix(shown, want) {
			t.Errorf("%s advertises %q, which does not end in %q — nothing answers that URL",
				purpose, shown, want)
		}
	}

	// And they are three distinct URLs. Registering one and assuming it covers
	// the others is the mistake this list exists to prevent.
	seen := map[string]bool{}
	for _, u := range s.redirectURIs {
		if seen[u.Url] {
			t.Errorf("two purposes advertise the same URL %q, so one of them is wrong", u.Url)
		}
		seen[u.Url] = true
	}
}

// The three sources the surface has to tell apart, and the one it used to get
// wrong: an app the DEPLOYMENT supplies is neither a stored app nor the absence
// of one, and reporting it as absent told operators Gmail could not be
// connected on installations where it demonstrably could.
func TestGoogleAppReportsWhichSourceTheConnectorWillActuallyUse(t *testing.T) {
	for name, tc := range map[string]struct {
		stored      bool
		storedID    string
		envClientID string
		wantSource  crmcontracts.GoogleAppSource
		wantID      string
		wantUsable  bool
	}{
		"nothing anywhere": {
			wantSource: crmcontracts.GoogleAppSourceNone, wantUsable: false,
		},
		"the deployment supplies one": {
			envClientID: "env-id", wantSource: crmcontracts.GoogleAppSourceEnvironment,
			wantID: "env-id", wantUsable: true,
		},
		// Stored wins, exactly as it does at the moment of a connect — anything
		// else would describe a resolution the connector does not perform.
		"a stored app wins over the deployment's": {
			stored: true, storedID: "stored-id", envClientID: "env-id",
			wantSource: crmcontracts.GoogleAppSourceStored, wantID: "stored-id", wantUsable: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			app := googleAppView(tc.stored, tc.storedID, tc.envClientID, nil)
			if app.Source != tc.wantSource {
				t.Errorf("source = %q, want %q", app.Source, tc.wantSource)
			}
			if app.ClientId != tc.wantID {
				t.Errorf("client id = %q, want %q", app.ClientId, tc.wantID)
			}
			if app.Configured != tc.wantUsable {
				t.Errorf("configured = %v, want %v — it answers whether Gmail can be connected at all", app.Configured, tc.wantUsable)
			}
			if app.RedirectUris == nil {
				t.Error("redirect_uris is null rather than an empty list; the field is contract-required")
			}
		})
	}
}
