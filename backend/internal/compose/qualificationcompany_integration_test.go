// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"maps"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAQualifiedDealBecomesAnExistingChoiceForTheNextLeadAtItsCompany(t *testing.T) {
	f := qualificationWithExistingDeal(t)
	first, err := f.contacts.QualifyLead(f.e.Admin(), f.lead, contacts.PromoteLeadInput{Trigger: "human_qualify", Deal: &contacts.QualifyDealInput{Name: "New requirements"}})
	if err != nil {
		t.Fatal(err)
	}
	if first.DealID == nil {
		t.Fatal("qualification opened no deal")
	}
	if f.e.WsCount(t, `SELECT count(*) FROM deal WHERE id=$1 AND company_id=$2`, first.DealID, f.company) != 1 {
		t.Fatal("new deal lost the contact's employer")
	}
	lead, _, err := f.contacts.CreateLead(f.e.Admin(), contacts.CreateLeadInput{FullName: new("Second buyer"), Email: new("second@renewal-buyer.test"), CompanyName: new("Renewal buyer"), CandidateCompanyKey: new("renewal-buyer.test"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.LeadKind](ids.UUID(lead.Id))
	if _, err := f.contacts.LinkLeadCompany(f.e.Admin(), id, f.company); err != nil {
		t.Fatal(err)
	}
	preview, err := f.contacts.PreviewLeadPromotion(f.e.Admin(), id)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	if preview.ExistingDeals != nil {
		for _, deal := range *preview.ExistingDeals {
			if ids.UUID(deal.Id) == *first.DealID {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("first buyer's deal absent from next qualification: %+v", preview)
	}
	before := f.e.WsCount(t, `SELECT count(*) FROM deal`)
	second, err := f.contacts.QualifyLead(f.e.Admin(), id, contacts.PromoteLeadInput{Trigger: "human_qualify", ExistingDealID: first.DealID})
	if err != nil {
		t.Fatal(err)
	}
	if second.DealID == nil || *second.DealID != *first.DealID || f.e.WsCount(t, `SELECT count(*) FROM deal`) != before {
		t.Fatal("second qualification duplicated the deal")
	}
}

func TestUndoingACreatedContactPreservesAnArchivedExistingDeal(t *testing.T) {
	f := qualificationWithExistingDeal(t)
	dealID := ids.UUID(f.deal.Id)
	out, err := f.contacts.QualifyLead(f.e.Admin(), f.lead, contacts.PromoteLeadInput{Trigger: "human_qualify", ExistingDealID: &dealID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.deals.ArchiveDeal(f.e.Admin(), ids.From[ids.DealKind](dealID), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.contacts.DemoteLead(f.e.Admin(), f.lead, "Wrong qualification"); err != nil {
		t.Fatal(err)
	}
	if f.e.WsCount(t, `SELECT count(*) FROM deal WHERE id=$1 AND archived_at IS NOT NULL`, dealID) != 1 {
		t.Fatal("undo changed the preexisting deal")
	}
	if f.e.WsCount(t, `SELECT count(*) FROM relationship WHERE kind='deal_stakeholder' AND contact_id=$1 AND deal_id=$2`, out.Contact.Id, dealID) != 1 {
		t.Fatal("undo deleted the stakeholder history")
	}
	if f.e.WsCount(t, `SELECT count(*) FROM lead WHERE id=$1 AND status='engaged' AND qualified_deal_id IS NULL AND archived_at IS NULL`, f.lead) != 1 {
		t.Fatal("undo did not restore the lead")
	}
}

func TestQualificationCandidateGrantGapsDoNotBreakTheContactPreview(t *testing.T) {
	f := qualificationWithExistingDeal(t)
	lead, _, err := f.contacts.CreateLead(f.e.Admin(), contacts.CreateLeadInput{FullName: new("Unlinked buyer"), Email: new("unlinked@renewal-buyer.test"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["company"] = principal.ObjectGrant{Read: true}
	preview, err := f.contacts.PreviewLeadPromotion(f.e.As(f.e.AdminUser, nil, perms), ids.From[ids.LeadKind](ids.UUID(lead.Id)))
	if err != nil {
		t.Fatal(err)
	}
	if preview.ExistingDeals != nil || preview.CompanyId != nil {
		t.Fatalf("candidate authority was bypassed: %+v", preview)
	}
}

func TestQualificationLetsTheEventFilerAttachEarlierLeadMailToTheChosenDeal(t *testing.T) {
	f := qualificationWithExistingDeal(t)
	f.e.GrantRole(t, f.e.AdminUser, "admin")
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	o := &owedEnv{e: f.e, owner: f.e.AdminUser, seat: seatAddress(t, f.e, f.e.AdminUser), now: now}
	mail := o.customerWrites(t, "buyer@renewal-buyer.test", "Please discuss our renewal", now.Add(-time.Hour))
	deal := ids.UUID(f.deal.Id)
	out, err := f.contacts.QualifyLead(f.e.Admin(), f.lead, contacts.PromoteLeadInput{Trigger: "human_qualify", ExistingDealID: &deal})
	if err != nil {
		t.Fatal(err)
	}
	integration.ApplyRiverSchema(t)
	filer := dealRefileFixture{o: o}
	args := filer.enqueue(t, "contact.created", ids.UUID(out.Contact.Id))
	filer.run(t, args)
	if f.e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=$1 AND deal_id=$2`, mail, deal) != 1 || f.e.WsCount(t, `SELECT count(*) FROM deal`) != 1 {
		t.Fatal("earlier lead mail did not settle on the chosen existing deal")
	}
	before := f.e.WsCount(t, `SELECT count(*) FROM audit_log`)
	filer.run(t, args)
	if f.e.WsCount(t, `SELECT count(*) FROM audit_log`) != before {
		t.Fatal("replayed attribution repeated the filing")
	}
}
