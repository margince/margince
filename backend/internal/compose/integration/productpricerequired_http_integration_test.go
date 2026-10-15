// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"maps"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// A product without a price would sell at 0 on every offer it joins. An
// explicit 0 is a free item and stays valid.
func TestAProductCreateWithoutAPriceIsRefusedOverHTTP(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	base := AnyMap{"name": "Seat", "currency": "EUR", "source": "manual"}

	for label, stated := range map[string]AnyMap{"absent": {}, "null": {"unit_price_minor": nil}} {
		t.Run(label, func(t *testing.T) {
			body := maps.Clone(base)
			maps.Copy(body, stated)
			var refusal namedRefusal
			if status := e.CallVerbatim(t, "POST", "/v1/products", body, nil, &refusal); status != http.StatusUnprocessableEntity {
				t.Fatalf("POST /v1/products %v → %d, want 422", body, status)
			}
			if !refusal.names("unit_price_minor") {
				t.Errorf("refusal names %+v, want field unit_price_minor", refusal.Details.Errors)
			}
		})
	}

	t.Run("free", func(t *testing.T) {
		body := maps.Clone(base)
		body["unit_price_minor"] = 0
		var product struct {
			UnitPriceMinor int64 `json:"unit_price_minor"`
		}
		if status := e.CallVerbatim(t, "POST", "/v1/products", body, nil, &product); status != http.StatusCreated {
			t.Fatalf("POST /v1/products with a price of 0 → %d, want 201", status)
		}
		if product.UnitPriceMinor != 0 {
			t.Errorf("unit_price_minor = %d, want 0", product.UnitPriceMinor)
		}
	})
}
