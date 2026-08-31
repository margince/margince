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

	"github.com/margince/margince/backend/internal/modules/capture/graph"
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

// canConnect reports whether the human-facing connect/callback transport can
// run: the sync creds plus a callback URL and a state key of at least
// minStateKeyLen bytes (a weak key is refused, not silently accepted).
func (c GraphConfig) canConnect() bool {
	return c.canSync() && len(c.StateKey) >= minStateKeyLen && c.PublicBaseURL != ""
}

// Enabled reports whether the connect/callback transport is fully configured —
// the same condition WithGraphCapture gates on, exported so a caller (cmd) can
// log accurately rather than guessing from the client id alone.
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

// WithGraphCapture wires the Microsoft Graph half of the connector OAuth
// transport (api role): it registers the graph connector on the connect
// registry — building the registry, signer, and base URLs if WithGmailCapture
// did not already (a graph-only deployment) — and installs the graph OAuth
// app the shared connect/callback dispatch resolves by provider. Like
// WithGmailCapture it requires the vault and a fully-configured app; absent
// either, the graph provider keeps its declared 501/422 by omission. Order:
// after WithKeyvault, and after WithGmailCapture when both are configured.
func WithGraphCapture(c GraphConfig) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		// The send pre-flight's fact, recorded BEFORE the transport gate below
		// and off canSync — the same split WithGmailCapture makes. An
		// installation holding client credentials but no state key mounts no
		// api-side connect transport and still sends perfectly well from the
		// worker, so gating this on canConnect would report it unable to.
		s.graphAppConfigured = c.canSync()
		if !c.canConnect() || s.vault == nil {
			return
		}
		if s.connectorHandlers.registry == nil {
			s.connectorHandlers.registry = NewCaptureRegistry(pool, s.vault, s.captureConfig)
			s.authority = identity.NewService(pool)
			s.signer = newStateSigner([]byte(c.StateKey))
			s.publicBaseURL = c.PublicBaseURL
			s.apiBaseURL = c.APIBaseURL
		}
		s.connectorHandlers.registry.Register(graph.New(newGraphOAuth(c), graph.NewAPI(nil, "")))
		s.graphOAuth = newGraphOAuth(c)
		s.graphAPI = graph.NewAPI(nil, "")
	}
}
