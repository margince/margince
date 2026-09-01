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
	"github.com/margince/margince/backend/internal/modules/capture"
)

// advertised returns the URL published for one purpose, and whether it is
// published at all.
func advertised(uris []crmcontracts.ConnectorAppRedirectUri, purpose crmcontracts.ConnectorAppRedirectUriPurpose) (string, bool) {
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

	shown, ok := advertised(s.composed[capture.AppProviderGoogle].redirectURIs, crmcontracts.SignIn)
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

// THE MICROSOFT TWIN, and the reason it is worth spelling twice rather than
// asserting the pair once: the two vendors publish their sign-in URI from
// different files, against different bases, and Microsoft matches the
// registered value byte for byte just as Google does — with a refusal
// (AADSTS50011) that names no URI, so a mismatch here is diagnosed by reading
// this test or not at all.
//
// It also settles what ONE Entra registration covers. Sign-in is published
// under the same AppProviderMicrosoft key the mailbox and calendar callbacks
// use, so the card lists all three against one app, and an operator who
// registers only what they see connecting mailboxes would break login.
func TestTheAdvertisedMicrosoftSignInRedirectIsTheOneTheFlowSends(t *testing.T) {
	var s Server
	WithMicrosoftSignIn(MicrosoftSignInConfig{
		ClientID: "cid", ClientSecret: "secret",
		Tenant:       "0f9c1b2a-3d4e-5f60-8a1b-2c3d4e5f6071",
		StateKey:     "0123456789012345678901234567890123",
		RedirectBase: "https://api.example.com", PostLoginURL: "/", FailureURL: "/#/login?oidc=failed",
	})(&s, nil)

	shown, ok := advertised(s.composed[capture.AppProviderMicrosoft].redirectURIs, crmcontracts.SignIn)
	if !ok {
		t.Fatal("a deployment that composed Microsoft sign-in advertises no sign-in redirect URI, so an operator is told to register nothing")
	}

	rec := httptest.NewRecorder()
	s.StartOidcSignIn(rec, httptest.NewRequest(http.MethodGet, "/v1/auth/oidc/microsoft/start", nil), "microsoft")
	location := rec.Header().Get("Location")
	if location == "" {
		t.Fatal("the start route sent no redirect, so there is nothing to compare the advertised URI against")
	}
	consent, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parsing the authorization request: %v", err)
	}
	if sent := consent.Query().Get("redirect_uri"); sent != shown {
		t.Errorf("the sign-in URI advertised to the operator is %q, but the authorization request sends %q.\n"+
			"Registering the advertised value would fail AADSTS50011.", shown, sent)
	}
}

// The case an operator is actually in when they need this: they are CREATING
// the OAuth client, so no client id exists yet. Withholding the URI until one
// does would hide it exactly when it is needed, and leave them guessing the one
// string Google matches byte for byte.
func TestTheSignInRedirectIsAdvertisedBeforeTheAppIsConfigured(t *testing.T) {
	var s Server
	WithGoogleSignIn(GoogleSignInConfig{RedirectBase: "https://api.example.com"})(&s, nil)

	shown, ok := advertised(s.composed[capture.AppProviderGoogle].redirectURIs, crmcontracts.SignIn)
	if !ok {
		t.Fatal("an operator with no Google app yet is told no sign-in URI to register, which is the one they need to create it")
	}
	// And it is the SAME string the composed flow will send once they finish.
	var configured Server
	WithGoogleSignIn(GoogleSignInConfig{
		ClientID: "cid", ClientSecret: "secret", StateKey: "0123456789012345678901234567890123",
		RedirectBase: "https://api.example.com", PostLoginURL: "/", FailureURL: "/#/login?oidc=failed",
	})(&configured, nil)
	willSend, _ := advertised(configured.composed[capture.AppProviderGoogle].redirectURIs, crmcontracts.SignIn)
	if shown != willSend {
		t.Errorf("the URI advertised before configuration is %q but after it is %q; an operator would register the wrong one", shown, willSend)
	}
}

// A deployment that knows no origin of its own advertises nothing, rather than a
// URL built on an empty base.
func TestNoSignInRedirectIsAdvertisedWithoutABase(t *testing.T) {
	var s Server
	WithGoogleSignIn(GoogleSignInConfig{ClientID: "cid"})(&s, nil)
	if _, ok := advertised(s.composed[capture.AppProviderGoogle].redirectURIs, crmcontracts.SignIn); ok {
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
		shown, ok := advertised(s.composed[capture.AppProviderGoogle].redirectURIs, purpose)
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
	for _, u := range s.composed[capture.AppProviderGoogle].redirectURIs {
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
		wantSource  crmcontracts.ConnectorAppSource
		wantID      string
		wantUsable  bool
	}{
		"nothing anywhere": {
			wantSource: crmcontracts.ConnectorAppSourceNone, wantUsable: false,
		},
		"the deployment supplies one": {
			envClientID: "env-id", wantSource: crmcontracts.ConnectorAppSourceEnvironment,
			wantID: "env-id", wantUsable: true,
		},
		// Stored wins, exactly as it does at the moment of a connect — anything
		// else would describe a resolution the connector does not perform.
		"a stored app wins over the deployment's": {
			stored: true, storedID: "stored-id", envClientID: "env-id",
			wantSource: crmcontracts.ConnectorAppSourceStored, wantID: "stored-id", wantUsable: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			app := connectorAppView(
				capture.AppProviderGoogle,
				capture.ConnectorAppStatus{Configured: tc.stored, ClientID: tc.storedID},
				connectorAppComposition{envClientID: tc.envClientID},
			)
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

// The environment's client id and the STORED app's directory are two different
// apps' facts. Pairing them would show an operator one app's id beside another's
// directory, and send them to check a registration that is not the one in use.
func TestAnEnvironmentAppIsNeverPairedWithAStoredDirectory(t *testing.T) {
	app := connectorAppView(
		capture.AppProviderMicrosoft,
		// Stored but half-configured: a directory recorded, no usable pair. The
		// environment is what the connector will actually use.
		capture.ConnectorAppStatus{Tenant: "stored-directory"},
		connectorAppComposition{envClientID: "env-entra"},
	)
	if app.Source != crmcontracts.ConnectorAppSourceEnvironment || app.ClientId != "env-entra" {
		t.Fatalf("app = %+v, want the environment's app in use", app)
	}
	if app.Tenant != nil {
		t.Errorf("tenant = %q beside an environment client id — two apps' facts in one answer", *app.Tenant)
	}
}

// Each Microsoft connect flow is a SEPARATE route, so an operator who registers
// only one gets AADSTS50011 on the other — and the key is `graph`/`graphcal`,
// never the `microsoft` the sign-in route uses.
func TestEveryMicrosoftBackedConnectorIsAdvertisedUnderItsOwnRouteKey(t *testing.T) {
	var s Server
	WithGraphCapture(GraphConfig{
		ClientID: "cid", ClientSecret: "secret",
		StateKey: "0123456789abcdef0123456789abcdef", PublicBaseURL: "https://app",
	})(&s, nil)

	uris := s.composed[capture.AppProviderMicrosoft].redirectURIs
	seen := map[crmcontracts.ConnectorAppRedirectUriPurpose]string{}
	for _, u := range uris {
		seen[u.Purpose] = u.Url
	}
	// Each URI is held to its OWN connector's callback path, not merely to
	// being present and different: two URIs that are distinct and both wrong
	// fail at Microsoft's consent screen exactly as one missing would.
	for _, want := range []struct {
		purpose crmcontracts.ConnectorAppRedirectUriPurpose
		suffix  string
	}{
		{crmcontracts.MailboxConnect, "/v1/connectors/" + providerGraph + "/callback"},
		{crmcontracts.CalendarConnect, "/v1/connectors/" + providerGraphCal + "/callback"},
	} {
		got := seen[want.purpose]
		if got == "" {
			t.Errorf("no %s URI advertised; registering the other one alone fails this flow at Microsoft's consent screen", want.purpose)
			continue
		}
		if !strings.HasSuffix(got, want.suffix) {
			t.Errorf("%s advertises %q, want it to end in %q — an operator registers this byte for byte", want.purpose, got, want.suffix)
		}
	}
	// And nothing that is Google's: one card per vendor, listing that vendor's
	// own URLs.
	if len(s.composed[capture.AppProviderGoogle].redirectURIs) != 0 {
		t.Errorf("the Microsoft option published %d Google URI(s)", len(s.composed[capture.AppProviderGoogle].redirectURIs))
	}
}
