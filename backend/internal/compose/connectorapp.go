// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Which OAuth app this installation connects with, and whether it can run the
// consent flow at all.
//
// Split from connectors.go because it answers a different question. That file is
// the connect/callback/disconnect TRANSPORT — one flow, provider-agnostic. This
// one is about configuration: the Google app may now arrive at runtime from a
// stored setting rather than the process environment, so "which credentials" and
// "are the prerequisites in place" stopped being facts settled at boot and became
// questions asked per request.

import (
	"context"
	"errors"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/gmail"
	"github.com/margince/margince/backend/internal/modules/capture/graph"
	"github.com/margince/margince/backend/internal/modules/capture/graphcal"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// writeConnectorAppFailure turns a credential-resolution failure into an answer
// the caller can act on.
//
// A missing vault is the operator's to fix and nothing the caller sent, so it
// reads as unavailable — the same 503 the write path answers, rather than two
// surfaces disagreeing about one condition. Anything else is a sealed secret
// that would not open, which is a real fault and keeps its 500.
func writeConnectorAppFailure(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, capture.ErrNoVault) {
		httperr.ServiceUnavailable(w, r, capture.ErrNoVault.Error())
		return
	}
	httperr.Write(w, r, err)
}

// oauthApp is one composed OAuth provider seen through the shared
// connect/callback flow: the consent-URL builder and the code-for-credential
// exchange, so the flow itself stays provider-agnostic.
type oauthApp struct {
	authCodeURL  func(state, redirectURI string) string
	authenticate func(ctx context.Context, code, redirectURI string) (connector.Auth, error)
}

// providerWired reports whether this deployment serves a provider's OAuth flow
// at all, without reading anything.
//
// Split from oauthApp so the session-less callback can answer its 501 before
// authenticating, and unseal a credential only after. The two questions are
// genuinely different: one is about how this process was composed, the other is
// about what the installation stored.
func (h connectorHandlers) providerWired(provider string) bool {
	switch provider {
	case providerGmail:
		return h.oauth != nil || (h.googleCredentials != nil && h.canRunConsent(providerGmail))
	case providerGcal:
		return h.gcalOAuth != nil || (h.googleCredentials != nil && h.canRunConsent(providerGcal))
	case providerGraph:
		return h.graphOAuth != nil || (h.microsoftCredentials != nil && h.canRunConsent(providerGraph))
	case providerGraphCal:
		return h.graphCalOAuth != nil || (h.microsoftCredentials != nil && h.canRunConsent(providerGraphCal))
	default:
		return false
	}
}

// canRunConsent reports whether this installation can actually run the consent
// flow for a provider with its STORED app.
//
// A stored app is only usable where BOTH halves exist: this transport to run the
// consent flow, and a registered connector for `Registry.Connect` to hand the
// credential to. The two are composed by different options, so a role can hold
// one without the other.
//
// Checked here so that gap answers the declared 501 at the gate, rather than
// letting a contact through the vendor's consent screen and failing afterwards
// with "connector not registered" — a refusal they cannot act on, having already
// granted access.
func (h connectorHandlers) canRunConsent(provider string) bool {
	// A usable signer first. The transport mounts on the deployment's state key,
	// so an unmounted one leaves this zero — and a stored app must not carry the
	// flow past the 501 into signing state with a key that never cleared the
	// floor.
	if !h.signer.usable() {
		return false
	}
	return h.hasConnector(provider)
}

// hasConnector reports whether name is registered on this role's capture
// registry.
func (h connectorHandlers) hasConnector(name string) bool {
	if h.registry == nil {
		return false
	}
	for _, d := range h.registry.Connectors() {
		if d.Name == name {
			return true
		}
	}
	return false
}

// connectorDescriptor returns the registered connector's own descriptor, or
// the zero value if name is not registered — the live source for anything
// that needs a connector's declared scopes without constructing a throwaway
// instance of it (e.g. one built with a nil dependency) just to read a
// static value.
func (h connectorHandlers) connectorDescriptor(name string) connector.Descriptor {
	if h.registry == nil {
		return connector.Descriptor{}
	}
	for _, d := range h.registry.Connectors() {
		if d.Name == name {
			return d
		}
	}
	return connector.Descriptor{}
}

// consentClient is the client one consent flow runs on, by resolveApp's rule.
// The environment's client comes first. The stored app is read only where this
// deployment can run the flow for it. The zero client is neither.
func consentClient[T comparable](
	ctx context.Context, env T, resolve appResolver, canRun bool, build func(capture.ConnectorApp) T,
) (T, error) {
	if !canRun {
		resolve = nil
	}
	return resolveApp(ctx, resolve, env, build, nil)
}

// connectResult is the closed vocabulary ListConnectors reports per provider
// and ConnectConnector's status code collapses to. The wire values live on
// CaptureProviderAvailability.Reason in crm.yaml; this is its Go mirror.
type connectResult string

const (
	// connectReady: a connect started right now would proceed.
	connectReady connectResult = "ready"
	// connectAppMissing: this installation has registered no OAuth app for the
	// vendor (or, for imap/an unrecognised provider, the registry itself is
	// not wired).
	connectAppMissing connectResult = "app_missing"
	// connectAppUnusable: an app IS stored and its secret would not open, which
	// is a different fix from registering one.
	connectAppUnusable connectResult = "app_unusable"
	// connectUnsupported: this deployment does not serve the provider at all.
	connectUnsupported connectResult = "unsupported"
)

// connectAvailability is what connectability reports for one provider.
type connectAvailability struct {
	reason connectResult
	// app is the resolved OAuth app, valid only when reason is connectReady
	// for an OAuth provider. Carried so ConnectConnector does not resolve
	// (and unseal) the same credential a second time after asking whether it
	// could.
	app oauthApp
	// err is the resolution failure behind an app_unusable reason, nil for
	// every other reason. ConnectConnector needs it, since writeConnectorAppFailure
	// keeps its 500-vs-503 distinction on it, but ListConnectors' roster has
	// no HTTP response to carry a status onto, so it reports the reason alone
	// and a per-provider failure there does not fail the read of the others.
	err error
}

// connectability reports whether a connect started right now would proceed
// for one provider: the one question ConnectConnector answers with a status
// code and ListConnectors answers with a reason on the roster. Both call this
// rather than each carrying its own copy of the decision, so a refusal a
// click gives can never surprise a screen that just reported readiness.
func (h connectorHandlers) connectability(ctx context.Context, provider string) connectAvailability {
	if provider == providerIMAP {
		// IMAP never needs an OAuth app: its credentials are per-connection,
		// vault-sealed, so the registry alone decides.
		if h.registry == nil {
			return connectAvailability{reason: connectUnsupported}
		}
		return connectAvailability{reason: connectReady}
	}
	if !isOAuthProvider(provider) {
		return connectAvailability{reason: connectUnsupported}
	}
	// Whether this DEPLOYMENT serves the provider at all, asked before anything
	// is read. It is a different question from what the installation stored,
	// and collapsing the two is how an admin who had just registered their app
	// was told they had not: without the OAuth transport (no state key, no
	// public base URL) a stored app cannot run its consent flow, so `oauthApp`
	// answers "none" for an installation that plainly has one — and the card
	// then sent them to Settings to register a second app against a gap no
	// setting there can close.
	if !h.providerWired(provider) {
		return connectAvailability{reason: connectUnsupported}
	}
	app, ok, err := h.oauthApp(ctx, provider)
	if err != nil {
		return connectAvailability{reason: connectAppUnusable, err: err}
	}
	if h.registry == nil || !ok {
		return connectAvailability{reason: connectAppMissing}
	}
	return connectAvailability{reason: connectReady, app: app}
}

// providerAvailability answers, for every mail provider the connect screen
// offers, whether a connect started right now would proceed.
//
// The per-provider resolution error is discarded here, not swallowed:
// connectability already classified it as app_unusable, which is all a roster
// entry can carry. There is no HTTP response on this path for
// ConnectConnector's own 500-vs-503 distinction to land on, and one provider's
// failure must not cost the reader the cards for the others.
// Returns the contract's own optional field rather than a bare slice: the
// field is optional so that an older server can stay silent, and this one
// never is, so the pointer is always the answer and never a decision the
// caller has to make.
func (h connectorHandlers) providerAvailability(ctx context.Context) *[]crmcontracts.CaptureProviderAvailability {
	out := make([]crmcontracts.CaptureProviderAvailability, 0, len(listedProviders))
	for _, p := range listedProviders {
		out = append(out, crmcontracts.CaptureProviderAvailability{
			Provider: crmcontracts.CaptureProviderAvailabilityProvider(p),
			Reason:   crmcontracts.CaptureProviderAvailabilityReason(h.connectability(ctx, p).reason),
		})
	}
	return &out
}

// oauthApp resolves the OAuth app for a provider; false when this installation
// has not configured it (its surface then answers the declared 501).
//
// Takes a context because the Google app is resolved from the DATABASE on each
// call. Building it at boot is what made an app set through Settings need a
// restart before Gmail could be connected — the credential is installation
// configuration, and asking for it when it is needed is the same move the model
// binding made when it stopped being a file.
func (h connectorHandlers) oauthApp(ctx context.Context, provider string) (oauthApp, bool, error) {
	switch provider {
	case providerGmail:
		return h.gmailApp(ctx)
	case providerGcal:
		return h.gcalApp(ctx)
	case providerGraph:
		return h.graphApp(ctx)
	case providerGraphCal:
		return h.graphCalApp(ctx)
	default:
		return oauthApp{}, false, nil
	}
}

// gmailApp builds the Gmail consent/exchange pair from the app this installation
// uses: the environment's where it composed one, the stored one otherwise.
func (h connectorHandlers) gmailApp(ctx context.Context) (oauthApp, bool, error) {
	// Nothing to resolve when neither source can serve this provider: no
	// env-composed client, and a stored app this installation cannot run the
	// consent flow for. Checked BEFORE the credential read so the refusal costs
	// no settings round-trip and does not unseal a secret that cannot be used.
	if h.oauth == nil && !h.canRunConsent(providerGmail) {
		return oauthApp{}, false, nil
	}
	oauth, err := consentClient(ctx, h.oauth, h.googleCredentials, h.canRunConsent(providerGmail),
		func(app capture.ConnectorApp) gmail.OAuth {
			return gmail.NewOAuth(gmail.OAuthConfig{
				ClientID: app.ClientID, ClientSecret: app.ClientSecretRef, Scopes: gmailScopes,
			})
		})
	if err != nil {
		return oauthApp{}, false, err
	}
	if oauth == nil {
		return oauthApp{}, false, nil
	}
	return oauthApp{
		authCodeURL: oauth.AuthCodeURL,
		authenticate: func(ctx context.Context, code, redirectURI string) (connector.Auth, error) {
			req, err := gmail.AuthRequestFrom(code, redirectURI)
			if err != nil {
				return nil, err
			}
			return gmail.New(oauth, h.gmailAPI).Authenticate(ctx, req)
		},
	}, true, nil
}

// gcalApp is the calendar half of the SAME Google app — one app per
// installation, so it resolves the same way. Leaving calendar on the environment
// while mail moved to the stored app is how the two would come to disagree about
// which Google project this installation is.
//
// Spelled separately rather than shared with gmailApp: gcal has its own OAuth
// type and config, so a helper covering both would be a generic over two type
// families to save a conditional. What they DO share — the credential lookup and
// the consent precondition — they share.
func (h connectorHandlers) gcalApp(ctx context.Context) (oauthApp, bool, error) {
	// Nothing to resolve when neither source can serve this provider: no
	// env-composed client, and a stored app this installation cannot run the
	// consent flow for. Checked BEFORE the credential read so the refusal costs
	// no settings round-trip and does not unseal a secret that cannot be used.
	if h.gcalOAuth == nil && !h.canRunConsent(providerGcal) {
		return oauthApp{}, false, nil
	}
	oauth, err := consentClient(ctx, h.gcalOAuth, h.googleCredentials, h.canRunConsent(providerGcal),
		func(app capture.ConnectorApp) gcal.OAuth {
			return gcal.NewOAuth(gcal.OAuthConfig{ClientID: app.ClientID, ClientSecret: app.ClientSecretRef})
		})
	if err != nil {
		return oauthApp{}, false, err
	}
	if oauth == nil {
		return oauthApp{}, false, nil
	}
	return oauthApp{
		authCodeURL: oauth.AuthCodeURL,
		authenticate: func(ctx context.Context, code, redirectURI string) (connector.Auth, error) {
			req, err := gcal.AuthRequestFrom(code, redirectURI)
			if err != nil {
				return nil, err
			}
			return gcal.New(oauth, h.gcalAPI).Authenticate(ctx, req)
		},
	}, true, nil
}

// graphApp is the Microsoft mail half of the installation's Entra registration.
// It takes the environment's app where it composed one, the stored one
// otherwise, as the Google pair does.
func (h connectorHandlers) graphApp(ctx context.Context) (oauthApp, bool, error) {
	// Nothing to resolve when neither source can serve this provider. Checked
	// BEFORE the credential read so the refusal costs no settings round-trip and
	// does not unseal a secret that cannot be used.
	if h.graphOAuth == nil && !h.canRunConsent(providerGraph) {
		return oauthApp{}, false, nil
	}
	oauth, err := consentClient(ctx, h.graphOAuth, h.microsoftCredentials, h.canRunConsent(providerGraph),
		func(app capture.ConnectorApp) graph.OAuth {
			return graph.NewOAuth(graph.OAuthConfig{
				ClientID: app.ClientID, ClientSecret: app.ClientSecretRef,
				Tenant: app.Tenant, Scopes: graphScopes,
			})
		})
	if err != nil {
		return oauthApp{}, false, err
	}
	if oauth == nil {
		return oauthApp{}, false, nil
	}
	return oauthApp{
		authCodeURL: oauth.AuthCodeURL,
		authenticate: func(ctx context.Context, code, redirectURI string) (connector.Auth, error) {
			req, err := graph.AuthRequestFrom(code, redirectURI)
			if err != nil {
				return nil, err
			}
			return graph.New(oauth, h.graphAPI).Authenticate(ctx, req)
		},
	}, true, nil
}

// graphCalApp is the Microsoft CALENDAR authorization on the same registration —
// its own consent, requesting the calendar permission alone, so it resolves
// separately from the mailbox's even though one Entra app serves both.
func (h connectorHandlers) graphCalApp(ctx context.Context) (oauthApp, bool, error) {
	if h.graphCalOAuth == nil && !h.canRunConsent(providerGraphCal) {
		return oauthApp{}, false, nil
	}
	oauth, err := consentClient(ctx, h.graphCalOAuth, h.microsoftCredentials, h.canRunConsent(providerGraphCal),
		func(app capture.ConnectorApp) graphcal.OAuth {
			return graphcal.NewOAuth(graphcal.OAuthConfig{
				ClientID: app.ClientID, ClientSecret: app.ClientSecretRef, Tenant: app.Tenant,
			})
		})
	if err != nil {
		return oauthApp{}, false, err
	}
	if oauth == nil {
		return oauthApp{}, false, nil
	}
	return oauthApp{
		authCodeURL: oauth.AuthCodeURL,
		authenticate: func(ctx context.Context, code, redirectURI string) (connector.Auth, error) {
			req, err := graphcal.AuthRequestFrom(code, redirectURI)
			if err != nil {
				return nil, err
			}
			return graphcal.New(oauth, h.graphCalAPI).Authenticate(ctx, req)
		},
	}, true, nil
}
