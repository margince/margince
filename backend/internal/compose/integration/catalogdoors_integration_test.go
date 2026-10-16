// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// A passport keeps the catalog on MCP as it does on REST. One price rule
// refuses a product with no price on both doors. A product the tools write is
// the product the route then reads.
func TestAPassportKeepsTheCatalogThroughTheRecordTools(t *testing.T) {
	d := newTwoDoors(t, "catalog-doors", "read", "write")

	unpriced := map[string]any{"name": "Free by accident", "currency": "EUR"}
	var problem struct {
		Detail string `json:"detail"`
	}
	if status := d.e.Call(t, "POST", "/v1/products", AnyMap{"source": "manual", "name": "Free by accident", "currency": "EUR"},
		d.bearer, &problem); status != http.StatusUnprocessableEntity || !strings.Contains(problem.Detail, "unit_price_minor") {
		t.Fatalf("an unpriced product over REST → %d %q, want 422 naming unit_price_minor", status, problem.Detail)
	}
	if refusal := d.mcp.CallRefused(t, "create_record", map[string]any{"record_type": "product", "fields": unpriced}); !strings.Contains(refusal, "unit_price_minor") {
		t.Fatalf("an unpriced product over MCP was refused with %q, want the refusal to name unit_price_minor", refusal)
	}

	created := d.tool(t, "create_record", map[string]any{"record_type": "product", "fields": map[string]any{
		"name": "Onboarding day", "unit_price_minor": 120000, "currency": "EUR",
	}})
	id := recordIDOf(t, created)
	d.tool(t, "update_record", map[string]any{"record_type": "product", "id": id, "fields": map[string]any{"unit_price_minor": 99000}})

	var product struct {
		UnitPriceMinor int64   `json:"unit_price_minor"`
		ArchivedAt     *string `json:"archived_at"`
	}
	if err := json.Unmarshal(d.rest(t, "GET", "/v1/products/"+id, nil), &product); err != nil {
		t.Fatal(err)
	}
	if product.UnitPriceMinor != 99000 {
		t.Errorf("the route reads unit_price_minor %d after the tool's update, want 99000", product.UnitPriceMinor)
	}

	d.tool(t, "archive_record", map[string]any{"record_type": "product", "id": id})
	if err := json.Unmarshal(d.rest(t, "GET", "/v1/products/"+id, nil), &product); err != nil {
		t.Fatal(err)
	}
	if product.ArchivedAt == nil {
		t.Error("the route reads the product as live after the tool archived it")
	}
}

// recordIDOf reads the id a record tool answered with.
func recordIDOf(t *testing.T, answer json.RawMessage) string {
	t.Helper()
	var ref struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(answer, &ref); err != nil || ref.ID == "" {
		t.Fatalf("the tool's answer names no record id:\n%s", answer)
	}
	return ref.ID
}
