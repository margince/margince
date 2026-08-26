// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture/googleconn"
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
func authorizerFor(t *testing.T, resolve googleAppResolver, envApp string) (googleAppAuthorizer, *string) {
	t.Helper()
	used := new(string)
	a := googleAppAuthorizer{
		resolve: resolve,
		build:   func(id, _ string) googleconn.Authorizer { return recordingAuthorizer{clientID: id, used: used} },
	}
	if envApp != "" {
		a.env = recordingAuthorizer{clientID: envApp, used: used}
	}
	return a, used
}

// What an admin sets through Settings outranks the environment: the environment
// is how the pair ARRIVES, and the stored app is where it lives.
func TestTheStoredAppOutranksTheEnvironment(t *testing.T) {
	a, used := authorizerFor(t, func(context.Context) (string, string, bool, error) {
		return "stored-id", "stored-secret", true, nil
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
	a, used := authorizerFor(t, func(context.Context) (string, string, bool, error) {
		return "", "", false, nil
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
	a, used := authorizerFor(t, func(context.Context) (string, string, bool, error) {
		return "", "", false, boom
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
	resolve := googleAppResolver(func(context.Context) (string, string, bool, error) {
		return "stored-id", "stored-secret", true, nil
	})
	for _, c := range []struct {
		name    string
		resolve googleAppResolver
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
	s.googleAppResolver = func(context.Context) (string, string, bool, error) {
		return "stored-id", "stored-secret", true, nil
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
