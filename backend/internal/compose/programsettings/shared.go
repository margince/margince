// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package programsettings declares the boot settings the programs read for
// themselves, outside any module. They sit below cmd so that compose knows
// each program's keys. A margince.yaml shared by the api and the worker must
// not fail either one on a key only the other reads.
//
// A key is the flag with `_` for `-`, the name an operator already types.
// Secret marks a credential: a DSN, password, client secret, token or key, and
// a URL that carries one. Any other URL, id, address or list stays visible.
package programsettings

import (
	"errors"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

var (
	apiOnly    = []config.Role{config.RoleAPI}
	workerOnly = []config.Role{config.RoleWorker}
	both       = []config.Role{config.RoleAPI, config.RoleWorker}
)

// The settings the api and the worker both read.
var (
	Redis = config.Define(config.Spec[string]{
		Key: "redis", Env: "MARGINCE_REDIS", Flag: "redis", Type: config.NonEmpty, Default: config.Some("localhost:16379"), Roles: both,
		Doc: "Redis address of the event bus.",
	})
	RedisPassword = config.Define(config.Spec[string]{
		Key: "redis_password", Env: "MARGINCE_REDIS_PASSWORD", Flag: "redis-password", Type: config.String, Secret: true, Roles: both,
		Doc: "Event-bus credential, where the Redis server requires one.",
	})
	AIRouting = config.Define(config.Spec[string]{
		Key: "ai_routing", Env: "MARGINCE_AI_ROUTING", Flag: "ai-routing", Type: config.String, Roles: both,
		Doc: "Ignored; kept so existing command lines parse.",
	})
	AIFake = config.Define(config.Spec[bool]{
		Key: "ai_fake", Env: "MARGINCE_AI_FAKE", Flag: "ai-fake", Type: config.Bool, Default: config.Some(false), Roles: both,
		Doc: "Drive the AI surfaces with the offline fake model when nothing else can serve (dev and test only).",
	})
	LogLevel = config.Define(config.Spec[string]{
		Key: "log_level", Env: "MARGINCE_LOG_LEVEL", Flag: "log-level", Roles: both,
		Type: config.Enum("debug", "info", "warn", "error"), Default: config.Some("info"),
		Doc: "Log level.",
	})
	LogFormat = config.Define(config.Spec[string]{
		Key: "log_format", Env: "MARGINCE_LOG_FORMAT", Flag: "log-format", Roles: both,
		Type: config.Enum("text", "json"), Default: config.Some("text"),
		Doc: "Log format.",
	})
	PublicBaseURL = config.Define(config.Spec[string]{
		Key: "public_base_url", Env: "MARGINCE_PUBLIC_BASE_URL", Flag: "public-base-url", Type: config.String, Roles: both,
		Doc: "External scheme and host for the links a buyer sees and for the OAuth callback; required to send mail.",
	})
	WebhookKey = config.Define(config.Spec[string]{
		Key: "webhook_key", Env: "MARGINCE_WEBHOOK_KEY", Flag: "webhook-key", Type: config.String, Secret: true, Roles: both,
		Doc: "Base64 32-byte key sealing outbound-webhook signing secrets; unset, the webhook paths that change data answer 503 and the worker sends nothing.",
	})
	VATCheckBaseURL = config.Define(config.Spec[string]{
		Key: "vat_check_base_url", Env: "MARGINCE_VAT_CHECK_BASE_URL", Flag: "vat-check-base-url", Type: config.String, Roles: both,
		Doc: "VIES base URL, or public; set it on the api and the worker together, or a VAT check is never queued.",
	})
	GeocodeBaseURL = config.Define(config.Spec[string]{
		Key: "geocode_base_url", Env: "MARGINCE_GEOCODE_BASE_URL", Flag: "geocode-base-url", Type: config.String, Roles: both,
		Doc: "Nominatim base URL; unset or public uses the public service, off disables geocoding; give the api and the worker the same value, or a lookup is never queued.",
	})
)

// The mail connectors' app credentials, which the api's connect flow and the
// worker's sync poll both use.
var (
	GmailClientID = config.Define(config.Spec[string]{
		Key: "gmail_client_id", Env: "MARGINCE_GMAIL_CLIENT_ID", Flag: "gmail-client-id", Type: config.String, Roles: both,
		Doc: "Google OAuth client id of the Gmail capture connector.",
	})
	GmailClientSecret = config.Define(config.Spec[string]{
		Key: "gmail_client_secret", Env: "MARGINCE_GMAIL_CLIENT_SECRET", Flag: "gmail-client-secret", Type: config.String, Secret: true, Roles: both,
		Doc: "Google OAuth client secret of the Gmail capture connector.",
	})
	GraphClientID = config.Define(config.Spec[string]{
		Key: "graph_client_id", Env: "MARGINCE_GRAPH_CLIENT_ID", Flag: "graph-client-id", Type: config.String, Roles: both,
		Doc: "Microsoft Entra app id of the Outlook and Microsoft 365 capture connector.",
	})
	GraphClientSecret = config.Define(config.Spec[string]{
		Key: "graph_client_secret", Env: "MARGINCE_GRAPH_CLIENT_SECRET", Flag: "graph-client-secret", Type: config.String, Secret: true, Roles: both,
		Doc: "Microsoft client secret of the Outlook and Microsoft 365 capture connector.",
	})
	GraphTenant = config.Define(config.Spec[string]{
		Key: "graph_tenant", Env: "MARGINCE_GRAPH_TENANT", Flag: "graph-tenant", Type: config.String, Roles: both,
		Validate: oneGraphAuthority,
		Doc:      "Microsoft identity tenant of the capture consent endpoint and token refresh; unset means common.",
	})
)

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

// FakeModelFault refuses the offline fake model from any source but the flag
// in production. A fake left on in a deployment's environment or files would
// serve canned answers; a flag is typed per command, as the desktop launcher
// does.
func FakeModelFault(c *config.Config, posture runtimeenv.Environment) error {
	fake, origin := config.ValWithSource(c, AIFake)
	if !fake || origin.FromFlag() || posture != runtimeenv.Production {
		return nil
	}
	return fmt.Errorf("ai_fake from %s turns on the offline fake model, and %s is production: "+
		"unset it, or pass --ai-fake on the command line", origin, runtimeenv.EnvVar)
}

// Settings lists the programs' own settings; each one's Roles says which
// programs read it.
func Settings() []config.Any {
	return []config.Any{
		Redis, RedisPassword, AIRouting, AIFake, LogLevel, LogFormat, PublicBaseURL, WebhookKey,
		VATCheckBaseURL, GeocodeBaseURL, GmailClientID, GmailClientSecret, GraphClientID, GraphClientSecret, GraphTenant,
		SchemaDSN, Addr, InlineRelay, MCPAppsBaseURL, APIBaseURL, ConnectorStateKey, MetricsToken, MetricsAccess,
		TrustedProxies, GmailPushToken, GmailPushAudience, GmailPushServiceAccount, GmailJWKSURL, GraphPushToken,
		MicrosoftSignInTenant,
		DataTasksPace, GmailPubsubTopic, GraphNotificationURL, JobDrainWindow, CertLogBaseURL, ObserveAddr, ObservePprof, AITaskDir,
	}
}
