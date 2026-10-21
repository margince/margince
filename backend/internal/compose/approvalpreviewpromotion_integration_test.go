// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestPromotionApprovalNamesTheContactAndOptionalDeal(t *testing.T) {
	for _, rest := range []bool{false, true} {
		t.Run(fmt.Sprintf("rest=%t", rest), func(t *testing.T) {
			e := integration.Setup(t)
			perms := integration.AdminPerms
			perms.Objects = maps.Clone(perms.Objects)
			perms.Objects["custom_field"] = principal.ObjectGrant{Read: true}
			ctx := e.As(e.AdminUser, nil, perms)
			dealStore := deals.NewStore(e.DB(), DealsInstallation())
			if err := dealStore.SeedDefaults(ctx); err != nil {
				t.Fatal(err)
			}
			lead, _, err := e.Contacts.CreateLead(ctx, contacts.CreateLeadInput{FullName: new("Dana Buyer"), Email: new("dana@example.test"), CompanyName: new("Example Buyer"), Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			payload := fmt.Sprintf(`{"lead_id":%q,"trigger":"human_qualify","evidence_note":"Buyer confirmed interest","deal":{"name":"Ignored by tool"}}`, lead.Id)
			if rest {
				payload = `{"operation":"PromoteLead","body":{"trigger":"human_qualify","evidence":{"note":"Buyer confirmed interest"},"deal":{"amount_minor":250000,"currency":"USD"}}}`
			}
			raw, hash, err := diffhash.Canonical([]byte(payload))
			if err != nil {
				t.Fatal(err)
			}
			svc := NewApprovalsService(e.DB())
			id, err := svc.Stage(ctx, approvals.StageInput{Kind: "promote_lead", TargetType: "lead", TargetID: ids.UUID(lead.Id), ProposedChange: raw, DiffHash: hash})
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := svc.GetWire(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			preview := proposal.Preview
			if preview == nil || preview.State != "ready" || preview.Version != 2 || preview.Details == nil || preview.Effects == nil {
				t.Fatalf("promotion preview: %+v", preview)
			}
			details := map[string]string{}
			for _, detail := range *preview.Details {
				details[detail.Field] = detail.Value.Text
			}
			if details["promotion_contact_name"] != "Dana Buyer" || details["promotion_contact_email"] != "dana@example.test" || details["promotion_note"] != "Buyer confirmed interest" {
				t.Fatalf("contact and evidence: %+v", details)
			}
			if slices.Contains(*preview.Effects, "promote_open_deal_seat_contact") != rest {
				t.Fatalf("wrong door's deal effects: %+v", preview.Effects)
			}
			if rest && (details["promotion_deal_name"] != "Example Buyer" || details["promotion_deal_amount"] != "250000" || details["pipeline_id"] == "" || details["stage_id"] == "") {
				t.Fatalf("deal preview: %+v", details)
			}
			if e.WsCount(t, `SELECT count(*) FROM contact`) != 0 || e.WsCount(t, `SELECT count(*) FROM deal`) != 0 {
				t.Fatal("reading a preview wrote records")
			}
			input, err := promotionApprovalInput(proposal)
			if err != nil {
				t.Fatal(err)
			}
			out, err := e.Contacts.WithDealOpener(leadDealOpener{deals: dealStore}).QualifyLead(ctx, ids.From[ids.LeadKind](ids.UUID(lead.Id)), input)
			if err != nil {
				t.Fatal(err)
			}
			if out.Contact.FullName != details["promotion_contact_name"] || (out.DealID != nil) != rest {
				t.Fatalf("actual promotion: %+v", out)
			}
		})
	}
}
