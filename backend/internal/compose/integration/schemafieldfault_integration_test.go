// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

func TestSchemaRefusalsNameTheFieldThroughTheRealWriteRoutes(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	deal := apptest.CreateOpenDeal(t, e, apptest.DiscoverSeededPipeline(t, e))
	for _, body := range []AnyMap{
		{"priority": "bogus"},
		{"commercial_motion": "bogus"},
		{"amount_minor": int64(9007199254740993)},
		{"description": strings.Repeat("界", 20001)},
	} {
		for field := range body {
			t.Run("deal "+field, func(t *testing.T) {
				assertSchemaFieldRefusal(t, e, "PATCH", "/v1/deals/"+deal, body, map[string]string{"If-Match": "1"}, field)
			})
		}
	}
	for _, field := range []string{"billing_interval_months", "currency"} {
		body := AnyMap{"name": "Refused price", "unit_price_minor": 100, "currency": "EUR", "billing_model": "recurring", "billing_interval_months": 1}
		if field == "currency" {
			body[field] = "eur"
		} else {
			body[field] = 2
		}
		assertSchemaFieldRefusal(t, e, "POST", "/v1/products", body, nil, field)
	}
	var offer struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/deals/"+deal+"/offers", AnyMap{"currency": "EUR"}, nil, &offer); status != http.StatusCreated {
		t.Fatalf("create draft offer: %d", status)
	}
	assertSchemaFieldRefusal(t, e, "POST", "/v1/offers/"+offer.ID+"/line-items", AnyMap{
		"quantity": 1, "description": "Recurring service", "unit_price_minor": 100,
		"billing_model": "recurring", "billing_interval_months": 2, "interval_count": 1,
	}, nil, "billing_interval_months")
}

func assertSchemaFieldRefusal(t *testing.T, e *apptest.AppEnv, method, path string, body AnyMap, headers map[string]string, field string) {
	t.Helper()
	var problem struct {
		Detail string               `json:"detail"`
		Fields []httperr.FieldError `json:"fields"`
	}
	status := e.Call(t, method, path, body, headers, &problem)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("%s %s: %d %+v", method, path, status, problem)
	}
	for _, fault := range problem.Fields {
		if fault.Field == field {
			return
		}
	}
	t.Fatalf("%s %s: expected field %s, got %+v", method, path, field, problem)
}
