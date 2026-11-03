// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnApprovedExistingDealIsPreviewedAndReplayedOnBothDoors(t *testing.T) {
	for _, rest := range []bool{false, true} {
		t.Run(fmt.Sprintf("rest=%t", rest), func(t *testing.T) {
			p := newPinnedPromotion(t, rest, "human_qualify")
			company, err := p.e.Contacts.CreateCompany(p.decider, contacts.CreateCompanyInput{DisplayName: "Pinned Works", Source: "manual", Domains: []contacts.CompanyDomainInput{{Domain: p.domain, IsPrimary: true}}})
			if err != nil {
				t.Fatal(err)
			}
			cid := ids.From[ids.CompanyKind](ids.UUID(company.Id))
			if _, err := p.e.Contacts.LinkLeadCompany(p.decider, p.lead, cid); err != nil {
				t.Fatal(err)
			}
			pipeline, stage, _ := integration.DealFixture(t, p.e)
			ds := deals.NewStore(p.e.DB(), DealsInstallation())
			deal, err := ds.CreateDeal(p.decider, deals.CreateDealInput{Name: "Existing scope", CompanyID: &cid, PipelineID: pipeline, StageID: stage, Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			p.e.Contacts.WithDealOpener(leadDealOpener{deals: ds})
			payload := fmt.Sprintf(`{"lead_id":%q,"trigger":"human_qualify","existing_deal_id":%q}`, p.lead, deal.Id)
			if rest {
				payload = fmt.Sprintf(`{"operation":"PromoteLead","body":{"trigger":"human_qualify","existing_deal_id":%q}}`, deal.Id)
			}
			p.change, p.hash, err = diffhash.Canonical([]byte(payload))
			if err != nil {
				t.Fatal(err)
			}
			id := p.stage()
			p.requireReady(p.agent, id, "promote_seat_contact_on_existing_deal")
			preview := p.preview(p.agent, id)
			found := false
			if preview.Details != nil {
				for _, detail := range *preview.Details {
					if detail.Field == "promotion_existing_deal" && strings.Contains(detail.Value.Text, "Existing scope") {
						found = true
					}
				}
			}
			if !found {
				t.Fatalf("existing deal missing from approval: %+v", preview.Details)
			}
			if err := p.approve(id); err != nil {
				t.Fatal(err)
			}
			if err := p.release(id); err != nil {
				t.Fatal(err)
			}
			if p.e.WsCount(t, `SELECT count(*) FROM deal`) != 1 || p.e.WsCount(t, `SELECT count(*) FROM relationship WHERE kind='deal_stakeholder' AND deal_id=$1 AND archived_at IS NULL`, deal.Id) != 1 {
				t.Fatal("approved replay duplicated or lost the deal")
			}
		})
	}
}
