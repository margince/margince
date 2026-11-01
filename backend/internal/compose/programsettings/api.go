// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package programsettings

import (
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/httpserver"
)

// The two postures --metrics-access names.
const (
	MetricsAccessToken = "token"
	MetricsAccessOpen  = "open"
)

// The settings only the api reads.
var (
	SchemaDSN = config.Define(config.Spec[string]{
		Key: "schema_dsn", Env: "MARGINCE_SCHEMA_DSN", Flag: "schema-dsn", Type: config.String, Secret: true, Roles: apiOnly,
		Doc: "Postgres DSN of the owner role for custom-field schema changes; unset, those two operations answer 501.",
	})
	Addr = config.Define(config.Spec[string]{
		Key: "addr", Env: "MARGINCE_ADDR", Flag: "addr", Type: config.NonEmpty, Default: config.Some(":8080"), Roles: apiOnly,
		Doc: "Listen address of the HTTP server.",
	})
	InlineRelay = config.Define(config.Spec[bool]{
		Key: "inline_relay", Env: "MARGINCE_INLINE_RELAY", Flag: "inline-relay", Type: config.Bool, Default: config.Some(true), Roles: apiOnly,
		Doc: "Run the outbox relay in this process; false when cmd/worker runs it.",
	})
	MCPAppsBaseURL = config.Define(config.Spec[string]{
		Key: "mcp_apps_base_url", Env: "MARGINCE_MCP_APPS_BASE_URL", Flag: "mcp-apps-base-url", Type: config.String, Roles: apiOnly,
		Doc: "Origin the api reads the MCP App view documents from; defaults to --public-base-url.",
	})
	APIBaseURL = config.Define(config.Spec[string]{
		Key: "api_base_url", Env: "MARGINCE_API_BASE_URL", Flag: "api-base-url", Type: config.String, Roles: apiOnly,
		Doc: "External base of the api for the OAuth redirect_uri when the api and the SPA are on different origins; defaults to --public-base-url.",
	})
	ConnectorStateKey = config.Define(config.Spec[string]{
		Key: "connector_state_key", Env: "MARGINCE_CONNECTOR_STATE_KEY", Flag: "connector-state-key", Type: config.String, Secret: true, Roles: apiOnly,
		Doc: "HMAC key of at least 32 bytes signing the OAuth connect state and the questions an MCP tool asks in the chat; required for the Gmail and Graph connect flows.",
	})
	MetricsToken = config.Define(config.Spec[string]{
		Key: "metrics_token", Env: "MARGINCE_METRICS_TOKEN", Flag: "metrics-token", Type: config.String, Secret: true, Roles: apiOnly,
		Doc: "Bearer credential /metrics requires; unset, /metrics refuses every scrape unless --metrics-access=open.",
	})
	MetricsAccess = config.Define(config.Spec[string]{
		Key: "metrics_access", Env: "MARGINCE_METRICS_ACCESS", Flag: "metrics-access", Roles: apiOnly,
		Type: config.Enum(MetricsAccessToken, MetricsAccessOpen), Default: config.Some(MetricsAccessToken),
		Doc: "Who /metrics serves: token requires --metrics-token, open serves any client that reaches the port.",
	})
	TrustedProxies = config.Define(config.Spec[string]{
		Key: "trusted_proxies", Env: "MARGINCE_TRUSTED_PROXIES", Flag: "trusted-proxies", Type: config.String, Roles: apiOnly,
		Validate: validTrustedProxies,
		Doc:      "Comma-separated CIDR prefixes of the reverse proxies whose client address header the rate limits per IP trust.",
	})
)

// The api's half of the mail connectors: push delivery and Microsoft sign-in,
// which shares the Graph app.
var (
	GmailPushToken = config.Define(config.Spec[string]{
		Key: "gmail_push_token", Env: "MARGINCE_GMAIL_PUSH_TOKEN", Flag: "gmail-push-token", Type: config.String, Secret: true, Roles: apiOnly,
		Doc: "Shared secret on the Pub/Sub push URL; unset, POST /webhooks/gmail is absent.",
	})
	GmailPushAudience = config.Define(config.Spec[string]{
		Key: "gmail_push_audience", Env: "MARGINCE_GMAIL_PUSH_AUDIENCE", Flag: "gmail-push-audience", Type: config.String, Roles: apiOnly,
		Doc: "OIDC audience the Pub/Sub push subscription mints tokens for.",
	})
	GmailPushServiceAccount = config.Define(config.Spec[string]{
		Key: "gmail_push_service_account", Env: "MARGINCE_GMAIL_PUSH_SERVICE_ACCOUNT", Flag: "gmail-push-service-account", Type: config.String, Roles: apiOnly,
		Doc: "Google service account email that signs the Pub/Sub push OIDC tokens.",
	})
	GmailJWKSURL = config.Define(config.Spec[string]{
		Key: "gmail_jwks_url", Env: "MARGINCE_GMAIL_JWKS_URL", Flag: "gmail-jwks-url", Type: config.String, Roles: apiOnly,
		Doc: "Override of the Google OIDC JWKS URL, for test and dev.",
	})
	GraphPushToken = config.Define(config.Spec[string]{
		Key: "graph_push_token", Env: "MARGINCE_GRAPH_PUSH_TOKEN", Flag: "graph-push-token", Type: config.String, Secret: true, Roles: apiOnly,
		Doc: "Shared secret on the Graph change notice URL; unset, POST /webhooks/graph is absent.",
	})
	MicrosoftSignInTenant = config.Define(config.Spec[string]{
		Key: "microsoft_signin_tenant", Env: "MARGINCE_MICROSOFT_SIGNIN_TENANT", Flag: "microsoft-signin-tenant", Type: config.String, Roles: apiOnly,
		Doc: "Comma-separated Entra directory ids whose members may sign in; defaults to --graph-tenant when that names a directory.",
	})
)

// validTrustedProxies keeps httpserver's parser the one reading of the list;
// cmd/api parses the validated text again for the value.
func validTrustedProxies(raw string) error {
	_, err := httpserver.ParseTrustedProxies(raw)
	return err
}
