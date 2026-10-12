// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/installseam"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A stale product edit is refused in a sentence a reader can act on. The store
// wraps the skew as "apply product patch", which names code, not the record.
func TestAStaleProductEditIsRefusedInAReadersSentence(t *testing.T) {
	e := Setup(t)
	editor := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{"deal_desk"},
		Objects: map[string]principal.ObjectGrant{
			"product": {Create: true, Read: true, Update: true},
		},
		RowScope: principal.RowScopeAll,
	})
	created, err := e.Deals.CreateProduct(editor, deals.CreateProductInput{
		Name: "Onboarding workshop", UnitPriceMinor: 120_000, Currency: "EUR", Source: "manual",
	})
	if err != nil {
		t.Fatalf("create the product: %v", err)
	}
	id := ids.From[ids.ProductKind](ids.UUID(created.Id))
	renamed := "Onboarding workshop, two days"
	if _, err := e.Deals.UpdateProduct(editor, id, deals.UpdateProductInput{Name: &renamed}); err != nil {
		t.Fatalf("the edit that makes the first version stale: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/v1/products/"+created.Id.String(),
		strings.NewReader(`{"name":"Onboarding workshop, half day"}`)).WithContext(editor)
	req.Header.Set("If-Match", strconv.FormatInt(*created.Version, 10))
	deals.NewHandlers(e.DB(), installseam.Deals()).UpdateProduct(rec, req, created.Id,
		crmcontracts.UpdateProductParams{})

	if rec.Code != http.StatusConflict {
		t.Fatalf("a stale edit answered %d, want 409: %s", rec.Code, rec.Body.String())
	}
	var problem struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode the refusal: %v", err)
	}
	if problem.Code != "version_skew" {
		t.Errorf("code = %q, want version_skew", problem.Code)
	}
	if strings.Contains(problem.Detail, "patch") || !strings.Contains(problem.Detail, "Reload") {
		t.Errorf("the refusal reads %q, want the reader's sentence telling them to reload", problem.Detail)
	}
}
