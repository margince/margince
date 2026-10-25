// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package programsettings

import (
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// The margince.yaml switches a program reads at boot. They carry no flag: a
// deployment states them in the file it reviews, or in the environment.
var (
	PasswordLogin = config.Define(config.Spec[bool]{
		Key: "auth.password.enabled", Env: "MARGINCE_AUTH_PASSWORD_ENABLED", Type: config.Bool, Default: config.Some(true), Roles: apiOnly,
		Doc: "Email and password sign-in; an installation behind its own identity provider turns it off.",
	})
	CapturePayloads = config.Define(config.Spec[bool]{
		Key: "ai.capture_payloads", Env: "MARGINCE_AI_CAPTURE_PAYLOADS", Type: config.Bool, Default: config.Some(false), Roles: both,
		Doc: "Keep each AI call's request and answer in ai_call_payload, aged by its retention policy; set it on the api and the worker together.",
	})
	MCPConnector = config.Define(config.Spec[bool]{
		Key: "mcp.connector_enabled", Env: "MARGINCE_MCP_CONNECTOR_ENABLED", Type: config.Bool, Default: config.Some(false), Roles: apiOnly,
		Doc: "Serve /mcp and the OAuth authorization server to outside agent clients; needs --public-base-url.",
	})
	// The two ways an AI tool gets an OAuth client ID without an admin. False
	// holds a way off whatever the admin's switch says; a client an admin
	// registers by hand is always allowed while the connector is on.
	MCPDynamicRegistration = config.Define(config.Spec[bool]{
		Key: "mcp.client_registration.dynamic", Env: "MARGINCE_MCP_CLIENT_REGISTRATION_DYNAMIC", Type: config.Bool, Default: config.Some(true), Roles: apiOnly,
		Doc: "Let AI tools register their own OAuth client at /oauth/register (DCR); false keeps it off whatever the admin's switch in Settings > MCP says.",
	})
	MCPMetadataDocument = config.Define(config.Spec[bool]{
		Key: "mcp.client_registration.metadata_document", Env: "MARGINCE_MCP_CLIENT_REGISTRATION_METADATA_DOCUMENT", Type: config.Bool, Default: config.Some(true), Roles: apiOnly,
		Doc: "Accept a client ID that is the URL of the tool's published metadata document (CIMD); false keeps it off whatever the admin's switch in Settings > MCP says.",
	})
	AnalyticsPerformance = config.Define(config.Spec[bool]{
		Key: "analytics.performance_enabled", Env: "MARGINCE_ANALYTICS_PERFORMANCE_ENABLED", Type: config.Bool, Default: config.Some(false), Roles: both,
		Doc: "Ignored; kept so existing files load.",
	})
	ListsEnabled = config.Define(config.Spec[bool]{
		Key: "lists.enabled", Env: "MARGINCE_LISTS_ENABLED", Type: config.Bool, Default: config.Some(true), Roles: both,
		Doc: "Live Lists and Shortlists; false hides every list surface and deletes no list.",
	})
	// AllowDataReset is not inferred from MARGINCE_ENV: a deployment called
	// staging may hold real users, and erasing tenant data is stated, never
	// inferred. Nor is it a stored setting, which an admin could arm through
	// the API against their own tenant.
	AllowDataReset = config.Define(config.Spec[bool]{
		Key: "operations.allow_data_reset", Env: "MARGINCE_OPERATIONS_ALLOW_DATA_RESET", Type: config.Bool, Default: config.Some(false), Roles: both,
		Doc: "Arm POST /v1/admin/reset-data, which purges this installation's tenant data back to its first-boot state.",
	})
	// AllowTestMailbox stays inert until a human connects it, and a rep with a
	// real mailbox grant still sends through that one.
	AllowTestMailbox = config.Define(config.Spec[bool]{
		Key: "operations.allow_test_mailbox", Env: "MARGINCE_OPERATIONS_ALLOW_TEST_MAILBOX", Type: config.Bool, Default: config.Some(false), Roles: both,
		Doc: "Arm the test_mailbox connector, a no-network fake mailbox that captures and sends; never on a real deployment.",
	})
	RatesFxSource = config.Define(config.Spec[string]{
		Key: "rates.fx_source", Env: "MARGINCE_RATES_FX_SOURCE", Type: config.String, Roles: workerOnly,
		Doc: "Page the FX refresh reads rates from; unset reads a free source of ECB rates.",
	})
	RatesFxCurrencies = config.Define(config.Spec[[]string]{
		Key: "rates.fx_currencies", Env: "MARGINCE_RATES_FX_CURRENCIES", Type: config.List(config.String), Roles: workerOnly,
		Validate: distinctCurrencies,
		Doc:      "ISO 4217 codes the FX refresh offers for an empty rate sheet; unset offers USD, GBP and CHF.",
	})
)

// distinctCurrencies fails a typo at boot rather than as a symbol the FX
// source silently omits.
func distinctCurrencies(codes []string) error {
	seen := make(map[string]bool, len(codes))
	for _, c := range codes {
		code := strings.ToUpper(strings.TrimSpace(c))
		if !values.AssignableCurrency(code) {
			return fmt.Errorf("%q is not a 3-letter ISO 4217 code", c)
		}
		if seen[code] {
			return fmt.Errorf("%q is listed twice", code)
		}
		seen[code] = true
	}
	return nil
}

// FileSettings lists the margince.yaml switches above.
func FileSettings() []config.Any {
	return []config.Any{PasswordLogin, CapturePayloads, MCPConnector, MCPDynamicRegistration, MCPMetadataDocument, AnalyticsPerformance, ListsEnabled, AllowDataReset, AllowTestMailbox, RatesFxSource, RatesFxCurrencies}
}
