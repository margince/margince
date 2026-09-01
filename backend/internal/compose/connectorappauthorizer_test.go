// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/googleconn"
	"github.com/margince/margince/backend/internal/modules/capture/graph"
	"github.com/margince/margince/backend/internal/modules/capture/oauthflow"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// recordingAuthorizer stands in for a built OAuth client and reports which
// credentials it was built from, which is the fact every test here turns on.
type recordingAuthorizer struct {
	clientID string
	used     *string
}

func (r recordingAuthorizer) Exchange(context.Context, string, string) (oauthflow.TokenGrant, error) {
	*r.used = r.clientID
	return oauthflow.TokenGrant{RefreshToken: "refresh"}, nil
}

func (r recordingAuthorizer) AccessToken(context.Context, string) (string, error) {
	*r.used = r.clientID
	return "access", nil
}

// authorizerFor builds the subject with a recording client on each side, so a
// test can tell WHICH app answered rather than only that something did.
func authorizerFor(t *testing.T, resolve appResolver, envApp string) (googleAppAuthorizer, *string) {
	t.Helper()
	used := new(string)
	a := googleAppAuthorizer{
		resolve: resolve,
		build: func(app capture.ConnectorApp) googleconn.Authorizer {
			return recordingAuthorizer{clientID: app.ClientID, used: used}
		},
	}
	if envApp != "" {
		a.env = recordingAuthorizer{clientID: envApp, used: used}
	}
	return a, used
}

// What an admin sets through Settings outranks the environment: the environment
// is how the pair ARRIVES, and the stored app is where it lives.
func TestTheStoredAppOutranksTheEnvironment(t *testing.T) {
	a, used := authorizerFor(t, func(context.Context) (capture.ConnectorApp, bool, error) {
		return capture.ConnectorApp{ClientID: "stored-id", ClientSecretRef: "stored-secret"}, true, nil
	}, "env-id")

	if _, err := a.AccessToken(context.Background(), "refresh"); err != nil {
		t.Fatalf("AccessToken: %v", err)
	}
	if *used != "stored-id" {
		t.Errorf("refreshed against %q, want the STORED app; the environment is not the authority once an app is set", *used)
	}
}

// An installation that never stored one keeps running on what the deployment
// composed, with no action required of its operator.
func TestTheEnvironmentStillServesWhenNothingIsStored(t *testing.T) {
	a, used := authorizerFor(t, func(context.Context) (capture.ConnectorApp, bool, error) {
		return capture.ConnectorApp{ClientID: "", ClientSecretRef: ""}, false, nil
	}, "env-id")

	if _, err := a.Exchange(context.Background(), "code", "https://app/cb"); err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if *used != "env-id" {
		t.Errorf("exchanged against %q, want the environment's app", *used)
	}
}

// A sealed secret that will not open means the vault's root key moved under the
// installation. Connecting anyway with an older environment copy would hide
// that behind a flow that half works.
func TestAResolutionFailureIsReportedNotPaperedOverWithTheEnvironment(t *testing.T) {
	boom := errors.New("vault: sealed secret will not open")
	a, used := authorizerFor(t, func(context.Context) (capture.ConnectorApp, bool, error) {
		return capture.ConnectorApp{}, false, boom
	}, "env-id")

	_, err := a.AccessToken(context.Background(), "refresh")
	if !errors.Is(err, boom) {
		t.Fatalf("AccessToken err = %v, want the resolution failure surfaced", err)
	}
	if *used != "" {
		t.Errorf("fell back to %q after a resolution error, hiding a broken vault behind a working-looking flow", *used)
	}
}

// With neither source the connector refuses in a way the registry PARKS on: no
// amount of backoff produces an app nobody configured.
func TestNoAppAnywhereRefusesAsRejectedRatherThanRetryable(t *testing.T) {
	a, _ := authorizerFor(t, nil, "")

	_, err := a.AccessToken(context.Background(), "refresh")
	if !errors.Is(err, connector.ErrAuthRejected) {
		t.Errorf("AccessToken err = %v, want it to wrap connector.ErrAuthRejected so the connection parks instead of retrying forever", err)
	}
}

// The registration rule, asserted through what the registry actually holds.
//
// This is the defect the operator hit: the Google connectors were registered
// only where the ENVIRONMENT carried the pair, and the transport asks the
// registry whether a connector exists before it will run the consent flow. An
// installation that set its app through Settings was therefore refused with the
// declared 501 and had no way to connect Gmail at all.
func TestAStoredAppRegistersTheGoogleConnectors(t *testing.T) {
	resolve := appResolver(func(context.Context) (capture.ConnectorApp, bool, error) {
		return capture.ConnectorApp{ClientID: "stored-id", ClientSecretRef: "stored-secret"}, true, nil
	})
	for _, c := range []struct {
		name    string
		resolve appResolver
		cfg     GmailConfig
		want    bool
	}{
		{"stored app only", resolve, GmailConfig{}, true},
		{"environment only", nil, GmailConfig{ClientID: "id", ClientSecret: "sec"}, true},
		{"both", resolve, GmailConfig{ClientID: "id", ClientSecret: "sec"}, true},
		{"neither", nil, GmailConfig{}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := googleAppReachable(c.resolve, c.cfg); got != c.want {
				t.Errorf("googleAppReachable = %v, want %v", got, c.want)
			}
		})
	}
}

// End to end through the options the api actually applies: an app that exists
// only in the database has to carry a person all the way to Google's consent
// screen.
func TestAStoredAppCanRunTheConsentFlow(t *testing.T) {
	var s Server
	s.vault = fakeVault{}
	WithKeyvault(fakeVault{})(&s, nil)
	// WithKeyvault builds the real resolver over a nil pool; stand a working one
	// in its place so this test is about the WIRING, not about the store.
	s.googleAppResolver = func(context.Context) (capture.ConnectorApp, bool, error) {
		return capture.ConnectorApp{ClientID: "stored-id", ClientSecretRef: "stored-secret"}, true, nil
	}
	// Only what a deployment must supply. No client id or secret anywhere in the
	// environment — the operator's case.
	WithGmailCapture(GmailConfig{
		StateKey: "0123456789abcdef0123456789abcdef", PublicBaseURL: "https://app",
	}, CaptureConfig{})(&s, nil)

	for _, provider := range []string{providerGmail, providerGcal} {
		t.Run(provider, func(t *testing.T) {
			if !s.providerWired(provider) {
				t.Fatalf("%s answers its declared 501 with an app stored; the installation cannot connect it at all", provider)
			}
			app, ok, err := s.oauthApp(context.Background(), provider)
			if err != nil || !ok {
				t.Fatalf("oauthApp(%s) ok=%v err=%v, want the stored app", provider, ok, err)
			}
			url := app.authCodeURL("state", "https://app/cb")
			if !strings.Contains(url, "stored-id") {
				t.Errorf("consent URL = %q, want it built from the STORED client id", url)
			}
		})
	}
}

// recordingGraphAuthorizer is the Microsoft twin: it reports which app it was
// built from, and it carries the third method the Google pair does not.
type recordingGraphAuthorizer struct {
	clientID, tenant string
	used             *string
	usedTenant       *string
}

func (r recordingGraphAuthorizer) Exchange(context.Context, string, string) (oauthflow.TokenGrant, error) {
	*r.used, *r.usedTenant = r.clientID, r.tenant
	return oauthflow.TokenGrant{RefreshToken: "refresh"}, nil
}

func (r recordingGraphAuthorizer) AccessToken(context.Context, string) (string, error) {
	*r.used, *r.usedTenant = r.clientID, r.tenant
	return "access", nil
}

func (r recordingGraphAuthorizer) Refresh(context.Context, string, []string) (oauthflow.TokenRefresh, error) {
	*r.used, *r.usedTenant = r.clientID, r.tenant
	return oauthflow.TokenRefresh{AccessToken: "access"}, nil
}

// graphAuthorizerFor builds the Microsoft subject with a recording client on
// each side.
func graphAuthorizerFor(t *testing.T, resolve appResolver, envApp string) (graphAppAuthorizer, *string, *string) {
	t.Helper()
	used, tenant := new(string), new(string)
	a := graphAppAuthorizer{
		resolve: resolve,
		build: func(app capture.ConnectorApp) graph.Authorizer {
			return recordingGraphAuthorizer{clientID: app.ClientID, tenant: app.Tenant, used: used, usedTenant: tenant}
		},
	}
	if envApp != "" {
		a.env = recordingGraphAuthorizer{clientID: envApp, tenant: "env-tenant", used: used, usedTenant: tenant}
	}
	return a, used, tenant
}

// All THREE methods resolve the same app. Refresh is the one the Google pair
// has no equivalent of, and a copy that resolved separately would refresh an
// Outlook mailbox against a different registration than it connected on.
func TestEveryMicrosoftMethodReachesTheSameResolvedApp(t *testing.T) {
	stored := func(context.Context) (capture.ConnectorApp, bool, error) {
		return capture.ConnectorApp{
			ClientID: "stored-entra", ClientSecretRef: "stored-secret", Tenant: "stored-tenant",
		}, true, nil
	}
	for name, call := range map[string]func(graphAppAuthorizer) error{
		"Exchange": func(a graphAppAuthorizer) error {
			_, err := a.Exchange(context.Background(), "code", "https://app/cb")
			return err
		},
		"AccessToken": func(a graphAppAuthorizer) error {
			_, err := a.AccessToken(context.Background(), "refresh")
			return err
		},
		"Refresh": func(a graphAppAuthorizer) error {
			_, err := a.Refresh(context.Background(), "refresh", []string{"Mail.Read"})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			a, used, tenant := graphAuthorizerFor(t, stored, "env-entra")
			if err := call(a); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if *used != "stored-entra" {
				t.Errorf("%s used %q, want the stored app", name, *used)
			}
			// The stored app carries its OWN directory: an operator who pinned
			// their registration to one directory pinned the app, and reading
			// the deployment's tenant over it would authorize a directory they
			// deliberately excluded.
			if *tenant != "stored-tenant" {
				t.Errorf("%s used tenant %q, want the stored app's own", name, *tenant)
			}
		})
	}
}

// With nothing stored the environment answers, and with neither the refusal
// names Microsoft rather than Google — the two apps are configured in different
// places and an operator sent to the wrong one finds nothing wrong there.
func TestTheMicrosoftRefusalNamesMicrosoft(t *testing.T) {
	a, used, _ := graphAuthorizerFor(t, nil, "env-entra")
	if _, err := a.AccessToken(context.Background(), "refresh"); err != nil {
		t.Fatalf("AccessToken: %v", err)
	}
	if *used != "env-entra" {
		t.Errorf("used %q, want the environment's app when nothing is stored", *used)
	}

	bare, _, _ := graphAuthorizerFor(t, nil, "")
	_, err := bare.AccessToken(context.Background(), "refresh")
	if !errors.Is(err, errNoMicrosoftApp) {
		t.Fatalf("AccessToken with no app = %v, want the Microsoft refusal", err)
	}
	if errors.Is(err, errNoGoogleApp) {
		t.Error("the Microsoft refusal is the Google one; an operator would be sent to the wrong console")
	}
	if !strings.Contains(err.Error(), "Microsoft") {
		t.Errorf("refusal = %q, want it to name the vendor", err)
	}
}
