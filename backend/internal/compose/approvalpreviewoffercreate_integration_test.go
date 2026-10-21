// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestOfferCreationApprovalMatchesTheWriter(t *testing.T) {
	for _, rest := range []bool{false, true} {
		t.Run(fmt.Sprintf("rest=%t", rest), func(t *testing.T) {
			e := integration.Setup(t)
			e.GrantRole(t, e.AdminUser, "admin")
			perms := integration.AdminPerms
			perms.Objects = maps.Clone(perms.Objects)
			for _, object := range []string{"offer", "product", "offer_template"} {
				perms.Objects[object] = principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
			}
			ctx := e.As(e.AdminUser, nil, perms)
			store := deals.NewStore(e.DB(), DealsInstallation())
			pipeline, err := store.CreatePipeline(ctx, deals.CreatePipelineInput{Name: "Sales"})
			if err != nil {
				t.Fatal(err)
			}
			stage, err := store.CreateStage(ctx, deals.CreateStageInput{PipelineID: ids.From[ids.PipelineKind](ids.UUID(pipeline.Id)), Name: "Discovery", Semantic: "open"})
			if err != nil {
				t.Fatal(err)
			}
			deal, err := store.CreateDeal(ctx, deals.CreateDealInput{Name: "Support renewal", PipelineID: ids.From[ids.PipelineKind](ids.UUID(pipeline.Id)), StageID: ids.From[ids.StageKind](ids.UUID(stage.Id)), Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			product, err := store.CreateProduct(ctx, deals.CreateProductInput{Name: "Quarterly support", UnitPriceMinor: 10000, Currency: "EUR", BillingModel: new("recurring"), BillingIntervalMonths: new(3), Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			template, err := store.CreateOfferTemplate(ctx, deals.CreateOfferTemplateInput{Name: "Standard terms"})
			if err != nil {
				t.Fatal(err)
			}
			terms := strings.Repeat("Complete agreement.\n", 30)
			fields := map[string]any{"currency": "EUR", "source": "manual", "template_id": template.Id, "terms_text": terms, "line_items": []map[string]any{{"product_id": product.Id, "quantity": 2, "interval_count": 2}}}
			body, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			var raw json.RawMessage
			var hash string
			if rest {
				raw, hash, err = canonicalRESTCall("createOffer", "/v1/deals/"+deal.Id.String()+"/offers", nil, body, keyBindsTheRetry)
			} else {
				fields["deal_id"] = deal.Id
				encoded, marshalErr := json.Marshal(map[string]any{"record_type": "offer", "fields": fields})
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				raw, hash, err = diffhash.Canonical(encoded)
			}
			if err != nil {
				t.Fatal(err)
			}
			svc := NewApprovalsService(e.DB())
			id, err := svc.Stage(ctx, approvals.StageInput{Kind: "create_record", TargetType: "offer", ProposedChange: raw, DiffHash: hash})
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := svc.GetWire(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if proposal.Preview == nil || proposal.Preview.State != "ready" || proposal.Preview.Details == nil {
				t.Fatalf("preview: %+v", proposal.Preview)
			}
			details := map[string]crmcontracts.ApprovalPreviewValue{}
			for _, detail := range *proposal.Preview.Details {
				details[detail.Field] = detail.Value
			}
			if details["deal_id"].Text != deal.Name || details["template_id"].Text != template.Name || details["product_id"].Text != product.Name || details["terms_text"].Text != terms {
				t.Fatalf("missing full context: %+v", details)
			}
			if details["arr_minor"].Kind != "money" || details["arr_minor"].Text != "80000" || details["net_tcv_minor"].Text != "40000" {
				t.Fatalf("incorrect recurring figures: %+v", details)
			}
			parent, input, err := offerCreationRequest(proposal)
			if err != nil {
				t.Fatal(err)
			}
			actual, err := store.CreateOffer(ctx, parent, input)
			if err != nil {
				t.Fatal(err)
			}
			if actual.DealId != deal.Id || actual.NetMinor == nil || *actual.NetMinor != 20000 || actual.TermsText == nil || *actual.TermsText != terms || actual.ArrMinor == nil || *actual.ArrMinor != 80000 {
				t.Fatalf("writer differs: %+v", actual)
			}
			delete(perms.Objects, "offer_template")
			hidden, err := svc.GetWire(e.As(e.AdminUser, nil, perms), id)
			if err != nil {
				t.Fatal(err)
			}
			if hidden.Preview == nil || hidden.Preview.State == "ready" || hidden.Preview.Details != nil {
				t.Fatalf("template disclosed without permission: %+v", hidden.Preview)
			}
		})
	}
}
