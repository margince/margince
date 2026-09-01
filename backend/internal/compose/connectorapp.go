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
// letting a person through the vendor's consent screen and failing afterwards
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
	if h.registry == nil {
		return false
	}
	for _, d := range h.registry.Connectors() {
		if d.Name == provider {
			return true
		}
	}
	return false
}

// storedApp resolves the installation's STORED app for one vendor.
//
// `stored` false means fall back to whatever the environment composed at boot:
// what an admin set through Settings outranks the environment, because the
// environment is how the pair ARRIVES and the stored app is where it lives.
//
// A resolution error is RETURNED, never turned into a fallback. A sealed secret
// that will not open means the vault's root key changed under the installation;
// quietly connecting with an older environment copy would hide that behind a
// flow that half works, and reporting it as "not configured" would send an
// operator to set up an app they already have.
func storedApp(ctx context.Context, resolve appResolver) (capture.ConnectorApp, bool, error) {
	if resolve == nil {
		return capture.ConnectorApp{}, false, nil
	}
	return resolve(ctx)
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
// uses — the stored one where it can run the flow, the environment's otherwise.
func (h connectorHandlers) gmailApp(ctx context.Context) (oauthApp, bool, error) {
	// Nothing to resolve when neither source can serve this provider: no
	// env-composed client, and a stored app this installation cannot run the
	// consent flow for. Checked BEFORE the credential read so the refusal costs
	// no settings round-trip and does not unseal a secret that cannot be used.
	if h.oauth == nil && !h.canRunConsent(providerGmail) {
		return oauthApp{}, false, nil
	}
	app, stored, err := storedApp(ctx, h.googleCredentials)
	if err != nil {
		return oauthApp{}, false, err
	}
	oauth := h.oauth
	if stored && h.canRunConsent(providerGmail) {
		oauth = gmail.NewOAuth(gmail.OAuthConfig{
			ClientID: app.ClientID, ClientSecret: app.ClientSecretRef, Scopes: gmailScopes,
		})
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
	app, stored, err := storedApp(ctx, h.googleCredentials)
	if err != nil {
		return oauthApp{}, false, err
	}
	oauth := h.gcalOAuth
	if stored && h.canRunConsent(providerGcal) {
		oauth = gcal.NewOAuth(gcal.OAuthConfig{ClientID: app.ClientID, ClientSecret: app.ClientSecretRef})
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

// graphApp is the Microsoft MAIL half of the installation's Entra registration —
// the stored app where it can run the flow, the environment's otherwise, exactly
// as the Google pair resolves.
func (h connectorHandlers) graphApp(ctx context.Context) (oauthApp, bool, error) {
	// Nothing to resolve when neither source can serve this provider. Checked
	// BEFORE the credential read so the refusal costs no settings round-trip and
	// does not unseal a secret that cannot be used.
	if h.graphOAuth == nil && !h.canRunConsent(providerGraph) {
		return oauthApp{}, false, nil
	}
	app, stored, err := storedApp(ctx, h.microsoftCredentials)
	if err != nil {
		return oauthApp{}, false, err
	}
	oauth := h.graphOAuth
	if stored && h.canRunConsent(providerGraph) {
		oauth = graph.NewOAuth(graph.OAuthConfig{
			ClientID: app.ClientID, ClientSecret: app.ClientSecretRef,
			Tenant: app.Tenant, Scopes: graphScopes,
		})
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
	app, stored, err := storedApp(ctx, h.microsoftCredentials)
	if err != nil {
		return oauthApp{}, false, err
	}
	oauth := h.graphCalOAuth
	if stored && h.canRunConsent(providerGraphCal) {
		oauth = graphcal.NewOAuth(graphcal.OAuthConfig{
			ClientID: app.ClientID, ClientSecret: app.ClientSecretRef, Tenant: app.Tenant,
		})
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
