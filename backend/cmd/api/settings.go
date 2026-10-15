// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"errors"
	"strings"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

// The api's boot settings. A key is the flag with `_` for `-`, the name an
// operator already types. Secret marks a credential only: a DSN, password,
// client secret, token or key. A URL, id, address or list stays visible, so a
// fault can quote it.
var (
	api = []config.Role{config.RoleAPI}

	apiDSN = config.Define(config.Spec[string]{
		Key: "dsn", Env: "MARGINCE_DSN", Flag: "dsn", Type: config.NonEmpty, Secret: true, Roles: api,
		Doc: "Postgres DSN of the runtime app role.",
	})
	apiSchemaDSN = config.Define(config.Spec[string]{
		Key: "schema_dsn", Env: "MARGINCE_SCHEMA_DSN", Flag: "schema-dsn", Type: config.String, Secret: true, Roles: api,
		Doc: "Postgres DSN of the owner role for custom-field schema changes; unset, those two operations answer 501.",
	})
	apiAddr = config.Define(config.Spec[string]{
		Key: "addr", Env: "MARGINCE_ADDR", Flag: "addr", Type: config.NonEmpty, Default: config.Some(":8080"), Roles: api,
		Doc: "Listen address of the HTTP server.",
	})
	apiRedis = config.Define(config.Spec[string]{
		Key: "redis", Env: "MARGINCE_REDIS", Flag: "redis", Type: config.NonEmpty, Default: config.Some("localhost:16379"), Roles: api,
		Doc: "Redis address of the event bus.",
	})
	apiRedisPassword = config.Define(config.Spec[string]{
		Key: "redis_password", Env: "MARGINCE_REDIS_PASSWORD", Flag: "redis-password", Type: config.String, Secret: true, Roles: api,
		Doc: "Event-bus credential, where the Redis server requires one.",
	})
	apiInlineRelay = config.Define(config.Spec[bool]{
		Key: "inline_relay", Env: "MARGINCE_INLINE_RELAY", Flag: "inline-relay", Type: config.Bool, Default: config.Some(true), Roles: api,
		Doc: "Run the outbox relay in this process; false when cmd/worker runs it.",
	})
	apiAIRouting = config.Define(config.Spec[string]{
		Key: "ai_routing", Env: "MARGINCE_AI_ROUTING", Flag: "ai-routing", Type: config.String, Roles: api,
		Doc: "Ignored; kept so existing command lines parse.",
	})
	apiAIFake = config.Define(config.Spec[bool]{
		Key: "ai_fake", Env: "MARGINCE_AI_FAKE", Flag: "ai-fake", Type: config.Bool, Default: config.Some(false), Roles: api,
		Doc: "Drive the AI surfaces with the offline fake model when nothing else can serve (dev and test only).",
	})
	apiLogLevel = config.Define(config.Spec[string]{
		Key: "log_level", Env: "MARGINCE_LOG_LEVEL", Flag: "log-level", Roles: api,
		Type: config.Enum("debug", "info", "warn", "error"), Default: config.Some("info"),
		Doc: "Log level.",
	})
	apiLogFormat = config.Define(config.Spec[string]{
		Key: "log_format", Env: "MARGINCE_LOG_FORMAT", Flag: "log-format", Roles: api,
		Type: config.Enum("text", "json"), Default: config.Some("text"),
		Doc: "Log format.",
	})
	apiPublicBaseURL = config.Define(config.Spec[string]{
		Key: "public_base_url", Env: "MARGINCE_PUBLIC_BASE_URL", Flag: "public-base-url", Type: config.String, Roles: api,
		Doc: "External scheme and host for buyer-facing links and the OAuth callback; required to send mail.",
	})
	apiMCPAppsBaseURL = config.Define(config.Spec[string]{
		Key: "mcp_apps_base_url", Env: "MARGINCE_MCP_APPS_BASE_URL", Flag: "mcp-apps-base-url", Type: config.String, Roles: api,
		Doc: "Origin the api reads the MCP App view documents from; defaults to --public-base-url.",
	})
	apiAPIBaseURL = config.Define(config.Spec[string]{
		Key: "api_base_url", Env: "MARGINCE_API_BASE_URL", Flag: "api-base-url", Type: config.String, Roles: api,
		Doc: "The api's external base for the OAuth redirect_uri when it is on another origin than the SPA; defaults to --public-base-url.",
	})
	apiConnectorStateKey = config.Define(config.Spec[string]{
		Key: "connector_state_key", Env: "MARGINCE_CONNECTOR_STATE_KEY", Flag: "connector-state-key", Type: config.String, Secret: true, Roles: api,
		Doc: "HMAC key of at least 32 bytes signing the OAuth connect state; required for the Gmail and Graph connect flows.",
	})
	apiWebhookKey = config.Define(config.Spec[string]{
		Key: "webhook_key", Env: "MARGINCE_WEBHOOK_KEY", Flag: "webhook-key", Type: config.String, Secret: true, Roles: api,
		Doc: "Base64 32-byte key sealing outbound-webhook signing secrets; unset, the mutating webhook paths answer 503.",
	})
	apiMetricsToken = config.Define(config.Spec[string]{
		Key: "metrics_token", Env: "MARGINCE_METRICS_TOKEN", Flag: "metrics-token", Type: config.String, Secret: true, Roles: api,
		Doc: "Bearer credential /metrics requires; unset, /metrics refuses every scrape unless --metrics-access=open.",
	})
	apiMetricsAccess = config.Define(config.Spec[string]{
		Key: "metrics_access", Env: "MARGINCE_METRICS_ACCESS", Flag: "metrics-access", Roles: api,
		Type: config.Enum(metricsAccessToken, metricsAccessOpen), Default: config.Some(metricsAccessToken),
		Doc: "Who /metrics serves: token requires --metrics-token, open serves anyone who reaches the port.",
	})
	apiTrustedProxies = config.Define(config.Spec[string]{
		Key: "trusted_proxies", Env: "MARGINCE_TRUSTED_PROXIES", Flag: "trusted-proxies", Type: config.String, Roles: api,
		Validate: validTrustedProxies,
		Doc:      "Comma-separated CIDR prefixes of the reverse proxies whose X-Forwarded-For per-IP rate limits believe.",
	})
	apiVATCheckBaseURL = config.Define(config.Spec[string]{
		Key: "vat_check_base_url", Env: "MARGINCE_VAT_CHECK_BASE_URL", Flag: "vat-check-base-url", Type: config.String, Roles: api,
		Doc: "VIES base URL; set it on the api and the worker together, or a VAT check is never queued.",
	})
	apiGeocodeBaseURL = config.Define(config.Spec[string]{
		Key: "geocode_base_url", Env: "MARGINCE_GEOCODE_BASE_URL", Flag: "geocode-base-url", Type: config.String, Roles: api,
		Doc: "Nominatim base URL; set it on the api and the worker together, or a lookup is never queued.",
	})
)

// The mail connectors' settings: Gmail and Microsoft Graph capture, and
// Microsoft sign-in, which shares the Graph app.
var (
	apiGmailClientID = config.Define(config.Spec[string]{
		Key: "gmail_client_id", Env: "MARGINCE_GMAIL_CLIENT_ID", Flag: "gmail-client-id", Type: config.String, Roles: api,
		Doc: "Google OAuth client id of the Gmail capture connector.",
	})
	apiGmailClientSecret = config.Define(config.Spec[string]{
		Key: "gmail_client_secret", Env: "MARGINCE_GMAIL_CLIENT_SECRET", Flag: "gmail-client-secret", Type: config.String, Secret: true, Roles: api,
		Doc: "Google OAuth client secret of the Gmail capture connector.",
	})
	apiGmailPushToken = config.Define(config.Spec[string]{
		Key: "gmail_push_token", Env: "MARGINCE_GMAIL_PUSH_TOKEN", Flag: "gmail-push-token", Type: config.String, Secret: true, Roles: api,
		Doc: "Shared secret on the Pub/Sub push URL; unset, POST /webhooks/gmail is absent.",
	})
	apiGmailPushAudience = config.Define(config.Spec[string]{
		Key: "gmail_push_audience", Env: "MARGINCE_GMAIL_PUSH_AUDIENCE", Flag: "gmail-push-audience", Type: config.String, Roles: api,
		Doc: "OIDC audience the Pub/Sub push subscription mints tokens for.",
	})
	apiGmailPushSA = config.Define(config.Spec[string]{
		Key: "gmail_push_service_account", Env: "MARGINCE_GMAIL_PUSH_SERVICE_ACCOUNT", Flag: "gmail-push-service-account", Type: config.String, Roles: api,
		Doc: "Google service account email that signs the Pub/Sub push OIDC tokens.",
	})
	apiGmailJWKSURL = config.Define(config.Spec[string]{
		Key: "gmail_jwks_url", Env: "MARGINCE_GMAIL_JWKS_URL", Flag: "gmail-jwks-url", Type: config.String, Roles: api,
		Doc: "Override of Google's OIDC JWKS URL, for test and dev.",
	})
	apiGraphClientID = config.Define(config.Spec[string]{
		Key: "graph_client_id", Env: "MARGINCE_GRAPH_CLIENT_ID", Flag: "graph-client-id", Type: config.String, Roles: api,
		Doc: "Microsoft Entra application id of the Outlook/M365 capture connector.",
	})
	apiGraphClientSecret = config.Define(config.Spec[string]{
		Key: "graph_client_secret", Env: "MARGINCE_GRAPH_CLIENT_SECRET", Flag: "graph-client-secret", Type: config.String, Secret: true, Roles: api,
		Doc: "Microsoft client secret of the Outlook/M365 capture connector.",
	})
	apiGraphPushToken = config.Define(config.Spec[string]{
		Key: "graph_push_token", Env: "MARGINCE_GRAPH_PUSH_TOKEN", Flag: "graph-push-token", Type: config.String, Secret: true, Roles: api,
		Doc: "Shared secret on the Graph change-notification URL; unset, POST /webhooks/graph is absent.",
	})
	apiGraphTenant = config.Define(config.Spec[string]{
		Key: "graph_tenant", Env: "MARGINCE_GRAPH_TENANT", Flag: "graph-tenant", Type: config.String, Roles: api,
		Validate: oneGraphAuthority,
		Doc:      "Microsoft identity tenant of the capture consent endpoint; unset means common.",
	})
	apiMicrosoftSignInTenant = config.Define(config.Spec[string]{
		Key: "microsoft_signin_tenant", Env: "MARGINCE_MICROSOFT_SIGNIN_TENANT", Flag: "microsoft-signin-tenant", Type: config.String, Roles: api,
		Doc: "Comma-separated Entra directory ids whose members may sign in; defaults to --graph-tenant when that names a directory.",
	})
)

// APISettings are the boot settings the api declares for itself.
func APISettings() []config.Any {
	return []config.Any{
		apiDSN, apiSchemaDSN, apiAddr, apiRedis, apiRedisPassword, apiInlineRelay, apiAIRouting, apiAIFake,
		apiLogLevel, apiLogFormat, apiPublicBaseURL, apiMCPAppsBaseURL, apiAPIBaseURL,
		apiConnectorStateKey, apiWebhookKey, apiMetricsToken, apiMetricsAccess, apiTrustedProxies,
		apiVATCheckBaseURL, apiGeocodeBaseURL,
		apiGmailClientID, apiGmailClientSecret, apiGmailPushToken, apiGmailPushAudience, apiGmailPushSA, apiGmailJWKSURL,
		apiGraphClientID, apiGraphClientSecret, apiGraphPushToken, apiGraphTenant, apiMicrosoftSignInTenant,
	}
}

// validTrustedProxies keeps httpserver's parser the one reading of the list;
// fromRegistry parses the validated text again for the value.
func validTrustedProxies(raw string) error {
	_, err := httpserver.ParseTrustedProxies(raw)
	return err
}

// oneGraphAuthority refuses a list: capture splices this value into
// login.microsoftonline.com/%s/oauth2/..., and its sibling
// --microsoft-signin-tenant, which does take a list, falls back to it.
func oneGraphAuthority(tenant string) error {
	if strings.Contains(tenant, ",") {
		return errors.New("--graph-tenant takes ONE authority (a directory id, or common) and got a list: " +
			tenant + " — several directories is a SIGN-IN posture, so put them in --microsoft-signin-tenant")
	}
	return nil
}

// legacyAPIItems are what the packages this role wires still declare as
// config.Item. Load does not read them; they are named so the unknown-variable
// report does not call them unread.
func legacyAPIItems() [][]config.Item {
	return [][]config.Item{
		blobstore.ConfigItems(),
		keyvault.ConfigItems(),
		ai.ConfigItems(),
		deployconfig.ConfigItems(),
		{{
			Name: compose.ProviderModeEnv, Kind: config.KindString, Default: "live",
			Roles: []string{config.LegacyRoleAPI, config.LegacyRoleWorker},
			Doc:   "enrichment provider: live|offline|off; an unknown value is a boot error rather than a silently disabled feature",
		}},
	}
}
