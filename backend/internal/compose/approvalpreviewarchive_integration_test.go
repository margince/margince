// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func TestCatalogArchiveApprovalsDescribeTheActualTargets(t *testing.T) {
	for _, rest := range []bool{false, true} {
		t.Run(fmt.Sprintf("rest=%t", rest), func(t *testing.T) {
			e := integration.Setup(t)
			perms := integration.AdminPerms
			perms.Objects = maps.Clone(perms.Objects)
			for _, object := range []string{"product", "offer_template", "offer", "activity"} {
				perms.Objects[object] = principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
			}
			ctx := e.As(e.AdminUser, nil, perms)
			store := deals.NewStore(e.DB(), DealsInstallation())
			product, err := store.CreateProduct(ctx, deals.CreateProductInput{Name: "Support", Currency: "EUR", UnitPriceMinor: 10000, Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			template, err := store.CreateOfferTemplate(ctx, deals.CreateOfferTemplateInput{Name: "Standard terms", IsDefault: true})
			if err != nil {
				t.Fatal(err)
			}
			pipeline, stage, _ := integration.DealFixture(t, e)
			deal, err := store.CreateDeal(ctx, deals.CreateDealInput{Name: "Expansion", PipelineID: pipeline, StageID: stage, Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			offer, err := store.CreateOffer(ctx, ids.From[ids.DealKind](ids.UUID(deal.Id)), deals.CreateOfferInput{Currency: "EUR", Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			if offer.OfferNumber == nil {
				t.Fatal("created offer has no number")
			}
			activity, _, err := activities.NewStore(e.DB()).LogActivity(ctx, activities.LogActivityInput{Kind: "task", Source: "manual", Subject: new("Review renewal")})
			if err != nil {
				t.Fatal(err)
			}
			cases := []struct {
				kind, op, path, effect, name string
				id                           ids.UUID
			}{
				{"activity", "archiveActivity", "activities", "archive_activity_preserve_content", "Review renewal", ids.UUID(activity.Id)},
				{"product", "archiveProduct", "products", "archive_product_preserve_snapshots", product.Name, ids.UUID(product.Id)},
				{"offer_template", "archiveOfferTemplate", "offer-templates", "archive_template_preserve_offers", template.Name, ids.UUID(template.Id)},
				{"offer", "archiveOffer", "offers", "archive_offer_preserve_lines", *offer.OfferNumber, ids.UUID(offer.Id)},
			}
			svc := NewApprovalsService(e.DB())
			for _, tc := range cases {
				payload := map[string]any{"record_type": tc.kind, "id": tc.id}
				if rest {
					payload = map[string]any{"operation": tc.op, "path": "/v1/" + tc.path + "/" + tc.id.String(), "body": nil}
				}
				encoded, err := json.Marshal(payload)
				if err != nil {
					t.Fatal(err)
				}
				raw, hash, err := diffhash.Canonical(encoded)
				if err != nil {
					t.Fatal(err)
				}
				id, err := svc.Stage(ctx, approvals.StageInput{Kind: "archive_record", TargetType: tc.kind, TargetID: tc.id, ProposedChange: raw, DiffHash: hash})
				if err != nil {
					t.Fatal(err)
				}
				proposal, err := svc.GetWire(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				p := proposal.Preview
				if p == nil || p.State != "ready" || p.Effects == nil || len(*p.Effects) != 1 || (*p.Effects)[0] != tc.effect || p.Details == nil {
					t.Fatalf("%s preview: %+v", tc.kind, p)
				}
				found := false
				for _, detail := range *p.Details {
					if detail.Value.Text == tc.name {
						found = true
					}
				}
				if !found {
					t.Fatalf("%s target name absent: %+v", tc.kind, *p.Details)
				}
				if proposal.TargetVersion == nil {
					t.Fatal("archive has no pin")
				}
				version := int64(*proposal.TargetVersion)
				if _, err := NewProviderFor(e.DB()).ArchiveAt(ctx, datasource.ArchiveInput{Ref: datasource.EntityRef{Type: datasource.EntityType(tc.kind), ID: tc.id}, IfVersion: &version}); err != nil {
					t.Fatal(err)
				}
				stale, err := svc.GetWire(ctx, id)
				if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
					t.Fatal(err)
				}
				if err == nil && (stale.Preview == nil || stale.Preview.State == "ready") {
					t.Fatalf("archived target still actionable: %+v", stale.Preview)
				}
				current, err := newApprovalPreviewReader(e.DB()).read(ctx, proposal)
				if err != nil || current == nil || current.State == "ready" {
					t.Fatalf("archived target projection: %+v / %v", current, err)
				}

			}
		})
	}
}
