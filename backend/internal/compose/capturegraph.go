// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The Microsoft (Graph) half of the connector OAuth surface.
//
// Split from capture.go because it is a second, independent provider app: one
// Microsoft app per deployment, its own tenant narrowing, its own scopes. It
// shares the transport in connectors.go and nothing else — and unlike the Google
// app it is still composed entirely from the deployment's environment, so it has
// no runtime-resolution half to keep in step.

import (
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/graph"
	"github.com/margince/margince/backend/internal/modules/capture/graphcal"
	"github.com/margince/margince/backend/internal/modules/identity"
)

// GraphConfig is the composed Microsoft (Graph) OAuth app for a deployment:
// one app per deployment, supplied by whoever operates it — the Microsoft
// twin of GmailConfig. ClientID+ClientSecret enable the background sync
// (token refresh); StateKey+PublicBaseURL additionally enable the
// connect/callback transport. Tenant narrows the identity endpoint to one
// Microsoft 365 tenant; empty means "common" (any organization).
type GraphConfig struct {
	ClientID     string
	ClientSecret string
	Tenant       string
	StateKey     string
	// PublicBaseURL is the canonical public/front origin (the SPA): the
	// post-consent landing, and the default callback base for a same-origin
	// deployment.
	PublicBaseURL string
	// APIBaseURL is the api's externally-reachable base, used only for the
	// callback redirect_uri. Empty for a same-origin deployment (the callback
	// rides PublicBaseURL); a split dev stack sets it to the api URL.
	APIBaseURL string
}

// canSync reports whether the connector can be registered + polled (token
// refresh needs the client id/secret).
func (c GraphConfig) canSync() bool { return c.ClientID != "" && c.ClientSecret != "" }

// canSignState reports what only a DEPLOYMENT can supply for the connect
// transport: a callback base and a state key of at least minStateKeyLen bytes (a
// weak key is refused, not silently accepted). The app itself is deliberately
// not part of it — it may arrive at runtime from the stored setting.
func (c GraphConfig) canSignState() bool {
	return len(c.StateKey) >= minStateKeyLen && c.PublicBaseURL != ""
}

// canConnect reports whether the human-facing connect/callback transport can run
// on the ENVIRONMENT's app alone: the sync creds plus what canSignState needs.
func (c GraphConfig) canConnect() bool { return c.canSync() && c.canSignState() }

// TransportMountable is the Microsoft twin of GmailConfig.TransportMountable —
// the DEPLOYMENT's half, mounted and logged on for the same reason, and with the
// same caveat about the vault it cannot see.
func (c GraphConfig) TransportMountable() bool { return c.canSignState() }

// Enabled reports whether this DEPLOYMENT composed a complete connect/callback
// transport of its own, exported so a caller (cmd) can log accurately rather
// than guessing from the client id alone.
//
// No longer the condition WithGraphCapture gates on: the option mounts on
// canSignState alone, because the app may arrive at runtime from the stored
// setting. So a false answer here means "the environment did not supply one",
// never "the surface is unavailable" — a boot line must say the first.
func (c GraphConfig) Enabled() bool { return c.canConnect() }

//nolint:ireturn // returns the graph.OAuth seam by design (a fakeable interface)
func newGraphOAuth(c GraphConfig) graph.OAuth {
	return graph.NewOAuth(graph.OAuthConfig{
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		Tenant:       c.Tenant,
		Scopes:       graphScopes,
	})
}

// newGraphCalOAuth builds the Microsoft CALENDAR authorization on the same app.
//
// A separate authorization requesting the calendar permission alone, exactly as
// the Google pair splits Gmail from Calendar: one consent each, so a person can
// bring their calendar without their mail, and disconnecting either leaves the
// other standing. The scopes are read from the graphcal package rather than
// restated here, so the consent requests exactly the permissions that connector
// declares.
//
//nolint:ireturn // returns the graphcal.OAuth seam by design (a fakeable interface)
func newGraphCalOAuth(c GraphConfig) graphcal.OAuth {
	return graphcal.NewOAuth(graphcal.OAuthConfig{
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		Tenant:       c.Tenant,
	})
}

// registerMicrosoftConnectors adds the Outlook pair where anything in this
// composition could supply the Entra app.
//
// Registering on neither source would leave a connector that fails every call
// with "no app configured", which is a worse answer than the declared 501: it
// looks configured and is not. Registering on EITHER is what lets an app stored
// through Settings be connected at all — the transport asks the registry whether
// a connector exists before it will run the consent flow.
func registerMicrosoftConnectors(
	reg *capture.Registry, resolve appResolver, g GraphConfig, pool *pgxpool.Pool,
) {
	if resolve == nil && !g.canSync() {
		return
	}
	reg.Register(graph.New(newGraphAuthorizer(resolve, g), graph.NewAPI(nil, "")).
		WithBounceSink(newBounceSink(pool)))
	reg.Register(graphcal.New(newGraphCalAuthorizer(resolve, g), graphcal.NewAPI(nil, "")))
}

// microsoftBackedConnectors are the connect flows one Entra registration serves,
// in the order an operator registers them. Each is a SEPARATE route under its own
// provider key, so registering one does not cover the other.
var microsoftBackedConnectors = []struct {
	purpose  crmcontracts.ConnectorAppRedirectUriPurpose
	provider string
}{
	{crmcontracts.MailboxConnect, providerGraph},
	{crmcontracts.CalendarConnect, providerGraphCal},
}

// WithGraphCapture wires the Microsoft Graph half of the connector OAuth
// transport (api role): it registers the graph connectors on the connect
// registry — building the registry, signer, and base URLs if WithGmailCapture
// did not already (a graph-only deployment) — and installs the graph OAuth app
// the shared connect/callback dispatch resolves by provider. Order: after
// WithKeyvault, and after WithGmailCapture when both are configured.
//
// What it requires is the VAULT and canSignState — a callback base and a state
// key — and deliberately not a configured app: the app may arrive at runtime
// from the stored setting, and an installation that has one must not be left
// with an unbuilt signer. Absent the vault or the signing prerequisites, the
// graph provider keeps its declared 501/422 by omission.
func WithGraphCapture(c GraphConfig) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		// The send pre-flight's fact, recorded BEFORE the transport gate below
		// and off canSync — the same split WithGmailCapture makes. An
		// installation holding client credentials but no state key mounts no
		// api-side connect transport and still sends perfectly well from the
		// worker, so gating this on canConnect would report it unable to.
		s.graphAppConfigured = c.canSync()
		if s.graphAppConfigured {
			s.composeEnvApp(capture.AppProviderMicrosoft, c.ClientID, c.Tenant)
			// OR, for the reason the Gmail option states: either vendor's
			// environment app finishes the setup step, and an installation
			// running on Microsoft alone is as set up as one running on Google.
			s.envConnectorApp = true
		}
		// Published BEFORE the gate below, for the same reason the Google pair
		// is: an operator registers these URLs while CREATING the Entra app,
		// which is before either flow can possibly work.
		for _, connector := range microsoftBackedConnectors {
			if uri := connectorCallbackURL(c.APIBaseURL, c.PublicBaseURL, connector.provider); uri != "" {
				s.addRedirectURI(capture.AppProviderMicrosoft, connector.purpose, uri)
			}
		}
		// The app's credentials are deliberately not part of this condition: they
		// may arrive at runtime from the stored setting, and an installation that
		// has one must not be left with an unbuilt signer. What stays required is
		// what only a deployment can supply.
		if !c.canSignState() || s.vault == nil {
			return
		}
		if s.connectorHandlers.registry == nil {
			s.connectorHandlers.registry = NewCaptureRegistry(pool, s.vault, s.captureConfig)
			s.authority = identity.NewService(pool)
			s.signer = newStateSigner([]byte(c.StateKey))
			s.publicBaseURL = c.PublicBaseURL
			s.apiBaseURL = c.APIBaseURL
		}
		registerMicrosoftConnectors(s.connectorHandlers.registry, s.microsoftAppResolver, c, pool)
		s.microsoftCredentials = s.microsoftAppResolver
		s.graphAPI = graph.NewAPI(nil, "")
		s.graphCalAPI = graphcal.NewAPI(nil, "")
		// The env-composed clients exist only where the ENVIRONMENT actually
		// carries the app. Built unconditionally they are usable-looking clients
		// holding an empty client id, and the fallback would reach for them the
		// moment the stored app is not servable — sending a person to Microsoft's
		// consent screen with `client_id=`, which fails there rather than here and
		// gives them nothing to act on.
		if c.canSync() {
			s.graphOAuth = newGraphOAuth(c)
			s.graphCalOAuth = newGraphCalOAuth(c)
		}
	}
}
