// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestProductCreationRequiresAnExplicitPrice(t *testing.T) {
	for _, rest := range []bool{false, true} {
		for _, price := range []struct {
			name    string
			present bool
			value   any
			allowed bool
		}{
			{name: "missing"}, {name: "null", present: true}, {name: "free", present: true, value: 0, allowed: true},
		} {
			t.Run(price.name, func(t *testing.T) {
				fields := map[string]any{"name": "Sample", "currency": "EUR", "source": "manual"}
				if price.present {
					fields["unit_price_minor"] = price.value
				}
				payload := map[string]any{"record_type": "product", "fields": fields}
				if rest {
					payload = map[string]any{"operation": "createProduct", "body": fields}
				}
				input, err := productCreationRequest(crmcontracts.Approval{TargetEntityType: new("product"), ProposedChange: &payload})
				if (err == nil) != price.allowed {
					t.Fatalf("rest=%t: allowed=%t, error=%v", rest, price.allowed, err)
				}
				if price.allowed && input.UnitPriceMinor != 0 {
					t.Fatalf("free item price: %d", input.UnitPriceMinor)
				}
			})
		}
	}
}
