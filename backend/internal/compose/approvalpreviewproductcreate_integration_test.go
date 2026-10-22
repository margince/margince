// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"fmt"
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestProductCreationApprovalMatchesTheWriter(t *testing.T) {
	for _, rest := range []bool{false, true} {
		t.Run(fmt.Sprintf("rest=%t", rest), func(t *testing.T) {
			e := integration.Setup(t)
			e.GrantRole(t, e.AdminUser, "admin")
			perms := integration.AdminPerms
			perms.Objects = maps.Clone(perms.Objects)
			perms.Objects["product"] = principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
			ctx := e.As(e.AdminUser, nil, perms)
			fields := map[string]any{"name": "  Support subscription  ", "sku": "SUPPORT", "source": "manual", "unit_price_minor": 25000, "currency": "EUR", "default_tax_rate": 19.12, "billing_model": "recurring", "billing_interval_months": 3}
			payload := map[string]any{"record_type": "product", "fields": fields}
			if rest {
				payload = map[string]any{"operation": "createProduct", "body": fields}
			}
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			raw, hash, err := diffhash.Canonical(encoded)
			if err != nil {
				t.Fatal(err)
			}
			svc := NewApprovalsService(e.DB())
			id, err := svc.Stage(ctx, approvals.StageInput{Kind: "create_record", TargetType: "product", ProposedChange: raw, DiffHash: hash})
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := svc.GetWire(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			preview := proposal.Preview
			if preview == nil || preview.State != "ready" || preview.Details == nil {
				t.Fatalf("preview: %+v", preview)
			}
			details := map[string]crmcontracts.ApprovalPreviewValue{}
			for _, detail := range *preview.Details {
				details[detail.Field] = detail.Value
			}
			if details["name"].Text != "Support subscription" || details["unit"].Text != "piece" || details["active"].Text != "true" || details["default_tax_rate"].Text != "19.12" || details["billing_model"].Text != "recurring" || details["billing_interval_months"].Text != "3" {
				t.Fatalf("defaults differ: %+v", details)
			}
			price := details["unit_price_minor"]
			if price.Kind != "money" || price.Text != "25000" || price.Currency == nil || *price.Currency != "EUR" {
				t.Fatalf("price: %+v", price)
			}
			if count := e.WsCount(t, `SELECT count(*) FROM product`); count != 0 {
				t.Fatalf("preview wrote %d products", count)
			}
			input, err := productCreationRequest(proposal)
			if err != nil {
				t.Fatal(err)
			}
			store := deals.NewStore(e.DB(), DealsInstallation())
			actual, err := store.CreateProduct(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			if actual.Name != details["name"].Text || actual.Unit != details["unit"].Text || !actual.Active || actual.DefaultTaxRate != 19.12 || actual.UnitPriceMinor != 25000 {
				t.Fatalf("writer differs: %+v", actual)
			}
			duplicate, err := svc.GetWire(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if duplicate.Preview == nil || duplicate.Preview.State != previewIncompleteState || duplicate.Preview.Details != nil {
				t.Fatalf("claimed SKU remains ready: %+v", duplicate.Preview)
			}
		})
	}
}
