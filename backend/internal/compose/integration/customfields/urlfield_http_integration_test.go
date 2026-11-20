// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package customfields

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestAURLFieldStoresAWebAddressAndRefusesAnythingElse(t *testing.T) {
	e := schemaWiredEnv(t)
	field := newField(t, e, integration.AnyMap{"object": "deal", "label": "Offer page", "type": "url", "source": "manual"})
	if field.Type != "url" {
		t.Fatalf("created field type = %q, want url", field.Type)
	}
	col := field.ColumnName
	stages := apptest.DiscoverSeededPipeline(t, e)
	_, id := createWithCF(t, e, "/v1/deals", integration.AnyMap{
		"name": "Linked offer", "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "source": "manual",
		col: "https://offers.example.com/42",
	})

	for _, refused := range []string{"javascript:alert(1)", "data:text/html,<b>x</b>", "offers.example.com/42", "https://user:secret@offers.example.com/"} {
		var problem integration.AnyMap
		if status := e.Call(t, "PATCH", "/v1/deals/"+id, integration.AnyMap{col: refused}, nil, &problem); status != http.StatusUnprocessableEntity {
			t.Errorf("PATCH %q → %d, want 422 (%v)", refused, status, problem)
		}
	}
	if got := readRecord(t, e, "/v1/deals/"+id)[col]; got != "https://offers.example.com/42" {
		t.Fatalf("a refused write changed the value to %v", got)
	}

	const upper = "HTTP://OFFERS.EXAMPLE.COM/43"
	if status := e.Call(t, "PATCH", "/v1/deals/"+id, integration.AnyMap{col: upper}, nil, nil); status != http.StatusOK {
		t.Fatalf("PATCH an upper-case http address → %d, want 200", status)
	}
	var page struct {
		Data []integration.AnyMap `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/deals?"+col+"="+url.QueryEscape(upper), nil, nil, &page); status != http.StatusOK {
		t.Fatalf("filter deals by the address → %d", status)
	}
	if len(page.Data) != 1 || page.Data[0]["id"] != id {
		t.Fatalf("filtering by the address found %d deals, want the one linked deal", len(page.Data))
	}
}
