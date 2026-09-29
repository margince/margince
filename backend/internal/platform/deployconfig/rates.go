// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deployconfig

import (
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// RatesConfig is the (worker-role) source config for the admin "Refresh from
// sources" jobs. Fx is the URL of a page the FX refresh fetches and AI-extracts
// rates from (defaults to api.frankfurter.dev when empty — read as page text,
// not parsed JSON); FxCurrencies is the candidate set the FX refresh proposes to
// bootstrap an empty sheet (worker default: USD/GBP/CHF).
// The FX refresh defaults both its source and candidate set, and no-ops when the
// model lane that extracts the page is absent. It never auto-applies — a human
// approves every staged proposal.
type RatesConfig struct {
	Fx           string   `yaml:"fx_source"`
	FxCurrencies []string `yaml:"fx_currencies"`
	// ModelPricing is still decoded, and deliberately: the file is parsed with
	// KnownFields(true), so deleting it would turn an upgrade into a refusal to
	// boot. Model prices now come from the broker's own catalogue; values here
	// are IGNORED and Warnings says so.
	ModelPricing map[string]string `yaml:"model_pricing"`
}

// Warnings names the settings this block still accepts but no longer acts on,
// one sentence each, for a role to log at boot.
func (r RatesConfig) Warnings() []string {
	if len(r.ModelPricing) == 0 {
		return nil
	}
	return []string{"rates.model_pricing is ignored: model prices are refreshed from the provider's own catalogue with Refresh model prices under Settings. Remove it from margince.yaml."}
}

// validate fails closed on a malformed candidate set: every fx_currencies entry
// must be an ISO 4217 code (the same shape company.base_currency is held
// to), and no currency may repeat. A typo must surface at boot, never as a
// silently dropped bootstrap symbol the FX source omits without a trace.
func (r RatesConfig) validate() error {
	seen := make(map[string]bool, len(r.FxCurrencies))
	for _, c := range r.FxCurrencies {
		code := strings.ToUpper(strings.TrimSpace(c))
		if !values.ValidCurrency(code) {
			return fmt.Errorf("deployconfig: rates.fx_currencies %q is not a 3-letter ISO 4217 code", c)
		}
		if seen[code] {
			return fmt.Errorf("deployconfig: rates.fx_currencies has a duplicate entry %q", code)
		}
		seen[code] = true
	}
	return nil
}
