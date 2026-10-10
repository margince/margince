// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The numbers and words an offer line and a product carry are held to the
// contract on every write, and a refusal names the field to fix.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type namedRefusal struct {
	Details struct {
		Errors []struct {
			Field string `json:"field"`
		} `json:"errors"`
	} `json:"details"`
}

func (f namedRefusal) names(field string) bool {
	for _, e := range f.Details.Errors {
		if e.Field == field {
			return true
		}
	}
	return false
}

func expectFieldRefused(t *testing.T, e *apptest.AppEnv, method, path string, body AnyMap, field string) {
	t.Helper()
	var refusal namedRefusal
	if status := e.Call(t, method, path, body, nil, &refusal); status != http.StatusUnprocessableEntity {
		t.Errorf("%s %s %v → %d, want 422", method, path, body, status)
		return
	}
	if !refusal.names(field) {
		t.Errorf("%s %s %v refusal names %+v, want field %q", method, path, body, refusal.Details.Errors, field)
	}
}

func TestAnOfferLineHoldsTheContractsFigures(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	offer := createOfferInCurrency(t, e, offerFixture(t, e), "EUR")
	lines := "/v1/offers/" + offer.ID + "/line-items"

	line := func(extra AnyMap) AnyMap {
		body := AnyMap{"description": "Support", "quantity": 1, "unit_price_minor": 100}
		for k, v := range extra {
			body[k] = v
		}
		return body
	}
	for field, bad := range map[string]AnyMap{
		"quantity":         {"quantity": 1.2345},
		"discount_pct":     {"discount_pct": 12.345},
		"tax_rate":         {"tax_rate": -1},
		"unit_price_minor": {"unit_price_minor": 9007199254740992},
		"position":         {"position": 0},
		"description":      {"description": "   "},
	} {
		expectFieldRefused(t, e, "POST", lines, line(bad), field)
	}
	expectFieldRefused(t, e, "POST", lines, line(AnyMap{"quantity": 0}), "quantity")
	expectFieldRefused(t, e, "POST", lines, line(AnyMap{"discount_pct": 100.001}), "discount_pct")

	var added offerBody
	if status := e.Call(t, "POST", lines, line(AnyMap{"quantity": 1.234, "discount_pct": 12.5}), nil, &added); status != http.StatusCreated {
		t.Fatalf("a line within its limits → %d, want 201", status)
	}
	one := lines + "/" + added.LineItems[len(added.LineItems)-1].ID
	for field, bad := range map[string]AnyMap{
		"description":  {"description": ""},
		"unit":         {"unit": "  "},
		"quantity":     {"quantity": 0.0004},
		"discount_pct": {"discount_pct": 101},
		"tax_rate":     {"tax_rate": 0.001},
	} {
		expectFieldRefused(t, e, "PATCH", one, bad, field)
	}
}

func TestAProductHoldsTheContractsFigures(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	product := func(extra AnyMap) AnyMap {
		body := AnyMap{"name": "Plan", "unit_price_minor": 100, "currency": "EUR", "source": "manual"}
		for k, v := range extra {
			body[k] = v
		}
		return body
	}
	for field, bad := range map[string]AnyMap{
		"unit_price_minor": {"unit_price_minor": -1},
		"default_tax_rate": {"default_tax_rate": -1},
		"currency":         {"currency": "eur"},
	} {
		expectFieldRefused(t, e, "POST", "/v1/products", product(bad), field)
	}

	var made AnyMap
	if status := e.Call(t, "POST", "/v1/products", product(nil), nil, &made); status != http.StatusCreated {
		t.Fatalf("create product → %d", status)
	}
	id, ok := made["id"].(string)
	if !ok {
		t.Fatalf("the created product carries no id: %v", made)
	}
	path := "/v1/products/" + id
	for field, bad := range map[string]AnyMap{
		"unit":             {"unit": "  "},
		"unit_price_minor": {"unit_price_minor": -1},
		"default_tax_rate": {"default_tax_rate": -0.5},
		"currency":         {"currency": "eur"},
	} {
		expectFieldRefused(t, e, "PATCH", path, bad, field)
	}
}
