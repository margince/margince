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
	RatesFxSource = config.Define(config.Spec[string]{
		Key: "rates.fx_source", Env: "MARGINCE_RATES_FX_SOURCE", Type: config.String, Roles: workerOnly,
		Doc: "Page the FX refresh reads rates from; unset reads api.frankfurter.dev.",
	})
	RatesFxCurrencies = config.Define(config.Spec[[]string]{
		Key: "rates.fx_currencies", Env: "MARGINCE_RATES_FX_CURRENCIES", Type: config.List(config.String), Roles: workerOnly,
		Validate: distinctCurrencies,
		Doc:      "ISO 4217 codes the FX refresh proposes for an empty rate sheet; unset proposes USD, GBP and CHF.",
	})
)

// distinctCurrencies fails a typo at boot rather than as a symbol the FX
// source silently omits.
func distinctCurrencies(codes []string) error {
	seen := make(map[string]bool, len(codes))
	for _, c := range codes {
		code := strings.ToUpper(strings.TrimSpace(c))
		if !values.ValidCurrency(code) {
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
	return []config.Any{PasswordLogin, CapturePayloads, RatesFxSource, RatesFxCurrencies}
}
