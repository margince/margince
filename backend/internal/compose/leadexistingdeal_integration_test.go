// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type existingQualification struct {
	e        *integration.Env
	contacts *contacts.Store
	deals    *deals.Store
	company  ids.CompanyID
	lead     ids.LeadID
	deal     crmcontracts.Deal
	pipeline crmcontracts.Pipeline
}

func qualificationWithExistingDeal(t *testing.T) existingQualification {
	t.Helper()
	e := integration.Setup(t)
	ds := deals.NewStore(e.DB(), DealsInstallation())
	if err := ds.SeedDefaults(e.Admin()); err != nil {
		t.Fatal(err)
	}
	cs := e.Contacts.WithDealOpener(leadDealOpener{deals: ds})
	company, err := cs.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Renewal buyer", Source: "manual", Domains: []contacts.CompanyDomainInput{{Domain: "renewal-buyer.test", IsPrimary: true}}})
	if err != nil {
		t.Fatal(err)
	}
	cid := ids.From[ids.CompanyKind](ids.UUID(company.Id))
	lead, _, err := cs.CreateLead(e.Admin(), contacts.CreateLeadInput{Email: new("buyer@renewal-buyer.test"), FullName: new("Renewal buyer"), CompanyName: new("Renewal buyer"), CandidateCompanyKey: new("renewal-buyer.test"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	lid := ids.From[ids.LeadKind](ids.UUID(lead.Id))
	if _, err := cs.LinkLeadCompany(e.Admin(), lid, cid); err != nil {
		t.Fatal(err)
	}
	pipeline, err := ds.DefaultPipeline(e.Admin())
	if err != nil {
		t.Fatal(err)
	}
	if pipeline.Stages == nil || len(*pipeline.Stages) == 0 {
		t.Fatal("default pipeline has no stages")
	}
	deal, err := ds.CreateDeal(e.Admin(), deals.CreateDealInput{Name: "Annual renewal", CompanyID: &cid, PipelineID: ids.From[ids.PipelineKind](ids.UUID(pipeline.Id)), StageID: ids.From[ids.StageKind](ids.UUID((*pipeline.Stages)[0].Id)), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	return existingQualification{e: e, contacts: cs, deals: ds, company: cid, lead: lid, deal: deal, pipeline: pipeline}
}

func TestQualificationLinksAnExistingDealWithoutOpeningAnother(t *testing.T) {
	f := qualificationWithExistingDeal(t)
	dealID := ids.UUID(f.deal.Id)
	preview, err := f.contacts.PreviewLeadPromotion(f.e.Admin(), f.lead)
	if err != nil || preview.ExistingDeals == nil || len(*preview.ExistingDeals) != 1 || (*preview.ExistingDeals)[0].Id != f.deal.Id {
		t.Fatalf("preview: %+v, %v", preview, err)
	}
	plan, err := f.contacts.PreviewLeadContactChange(f.e.Admin(), f.lead, contacts.PromoteLeadInput{Trigger: "human_qualify", ExistingDealID: &dealID})
	if err != nil || plan.ExistingDealID == nil || *plan.ExistingDealID != dealID || plan.Deal != nil {
		t.Fatalf("approval preview: %+v, %v", plan, err)
	}
	out, err := f.contacts.QualifyLead(f.e.Admin(), f.lead, contacts.PromoteLeadInput{Trigger: "human_qualify", ExistingDealID: &dealID})
	if err != nil {
		t.Fatal(err)
	}
	if out.DealID == nil || *out.DealID != dealID {
		t.Fatalf("deal result: %+v", out)
	}
	lead, err := f.contacts.GetLead(f.e.Admin(), f.lead, storekit.IncludeArchived)
	if err != nil || lead.QualifiedDealId == nil || ids.UUID(*lead.QualifiedDealId) != dealID {
		t.Fatalf("lead: %+v, %v", lead, err)
	}
	if f.e.WsCount(t, `SELECT count(*) FROM deal`) != 1 {
		t.Fatal("qualification opened another deal")
	}
	if f.e.WsCount(t, `SELECT count(*) FROM relationship WHERE kind='deal_stakeholder' AND contact_id=$1 AND deal_id=$2 AND archived_at IS NULL`, out.Contact.Id, dealID) != 1 {
		t.Fatal("missing stakeholder")
	}
	if f.e.WsCount(t, `SELECT count(*) FROM audit_log a JOIN relationship r ON r.id=a.entity_id WHERE a.entity_type='relationship' AND a.action='create' AND r.kind='deal_stakeholder' AND r.contact_id=$1 AND r.deal_id=$2`, out.Contact.Id, dealID) != 1 {
		t.Fatal("stakeholder audit is not unique")
	}
	if f.e.WsCount(t, `SELECT count(*) FROM event_outbox o JOIN audit_log a ON a.id::text=o.envelope->'trace'->>'audit_log_id' JOIN relationship r ON r.id=a.entity_id WHERE o.envelope->>'type'='deal.updated' AND a.entity_type='relationship' AND a.action='create' AND r.kind='deal_stakeholder' AND r.contact_id=$1 AND r.deal_id=$2`, out.Contact.Id, dealID) != 1 {
		t.Fatal("stakeholder event is not unique")
	}
	var hasDeal *contacts.ContactHasDealError
	if _, err := f.contacts.DemoteLead(f.e.Admin(), f.lead, "Wrong qualification"); !errors.As(err, &hasDeal) {
		t.Fatalf("undo of live stakeholder: %v", err)
	}
}

func TestQualificationRefusesConflictingDealChoicesBeforeWriting(t *testing.T) {
	f := qualificationWithExistingDeal(t)
	dealID := ids.UUID(f.deal.Id)
	_, err := f.contacts.QualifyLead(f.e.Admin(), f.lead, contacts.PromoteLeadInput{Trigger: "human_qualify", ExistingDealID: &dealID, Deal: &contacts.QualifyDealInput{}})
	fault, ok := httperr.Classify(err)
	if !ok || fault.Status != 422 || len(fault.Fields) != 2 {
		t.Fatalf("conflicting choices: %v", err)
	}
	if fault.Fields[0].Field != "deal" || fault.Fields[1].Field != "existing_deal_id" {
		t.Fatalf("conflict fields: %+v", fault.Fields)
	}
	if f.e.WsCount(t, `SELECT count(*) FROM contact WHERE converted_from_lead_id=$1`, f.lead) != 0 {
		t.Fatal("refused qualification created a contact")
	}
}

func TestQualificationRefusesAnUnavailableDealAndRollsBack(t *testing.T) {
	for _, state := range []string{"closed", "archived", "other company", "read only"} {
		t.Run(state, func(t *testing.T) {
			f := qualificationWithExistingDeal(t)
			ctx := f.e.Admin()
			switch state {
			case "closed":
				f.closeDeal(t)
			case "archived":
				if _, err := f.deals.ArchiveDeal(ctx, ids.From[ids.DealKind](ids.UUID(f.deal.Id)), nil); err != nil {
					t.Fatal(err)
				}
			case "other company":
				other, err := f.contacts.CreateCompany(ctx, contacts.CreateCompanyInput{DisplayName: "Other buyer", Source: "manual"})
				if err != nil {
					t.Fatal(err)
				}
				company := ids.From[ids.CompanyKind](ids.UUID(other.Id))
				if _, err := f.deals.UpdateDeal(ctx, ids.From[ids.DealKind](ids.UUID(f.deal.Id)), deals.UpdateDealInput{CompanyID: &company}); err != nil {
					t.Fatal(err)
				}
			case "read only":
				ctx = f.e.As(ids.NewV7(), nil, principal.Permissions{RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{"lead": {Read: true, Update: true}, "contact": {Read: true, Create: true, Update: true}, "company": {Read: true, Create: true}, "relationship": {Read: true, Create: true}, "deal": {Read: true}}})
			}
			id := ids.UUID(f.deal.Id)
			_, err := f.contacts.QualifyLead(ctx, f.lead, contacts.PromoteLeadInput{Trigger: "human_qualify", ExistingDealID: &id})
			want := apperrors.ErrNotFound
			if state == "read only" {
				want = apperrors.ErrPermissionDenied
			}
			if !errors.Is(err, want) {
				t.Fatalf("unavailable target: %v", err)
			}
			if f.e.WsCount(t, `SELECT count(*) FROM contact WHERE converted_from_lead_id=$1`, f.lead) != 0 {
				t.Fatal("refused qualification left a contact")
			}
			lead, err := f.contacts.GetLead(f.e.Admin(), f.lead, storekit.LiveOnly)
			if err != nil || lead.Status == crmcontracts.LeadStatusPromoted {
				t.Fatalf("lead was changed: %+v, %v", lead, err)
			}
		})
	}
}

func TestQualificationPreservesAPreexistingStakeholderAndItsProvenance(t *testing.T) {
	f := qualificationWithExistingDeal(t)
	ctx := f.e.Admin()
	contact, err := f.contacts.CreateContact(ctx, contacts.CreateContactInput{FullName: "Existing buyer", Source: "manual", Emails: []contacts.ContactEmailInput{{Email: "known@renewal-buyer.test", EmailType: "work", IsPrimary: true}}})
	if err != nil {
		t.Fatal(err)
	}
	contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	dealID := ids.From[ids.DealKind](ids.UUID(f.deal.Id))
	if _, err := f.contacts.CreateRelationship(ctx, contacts.CreateRelationshipInput{Kind: "deal_stakeholder", ContactID: &contactID, DealID: &dealID, Role: new("champion"), Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	// Historical leads may share a live contact's address; today's admission refuses that state.
	f.e.WsExec(t, `UPDATE lead SET email=$2 WHERE id=$1`, f.lead, "known@renewal-buyer.test")
	audits := f.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='relationship'`)
	id := dealID.UUID
	out, err := f.contacts.QualifyLead(ctx, f.lead, contacts.PromoteLeadInput{Trigger: "human_qualify", ExistingDealID: &id})
	if err != nil || !out.Merged || out.Contact.Id != contact.Id {
		t.Fatalf("existing-contact qualification: %+v, %v", out, err)
	}
	if f.e.WsCount(t, `SELECT count(*) FROM relationship WHERE kind='deal_stakeholder' AND contact_id=$1 AND deal_id=$2 AND source='manual' AND role='champion' AND archived_at IS NULL`, contactID, dealID) != 1 {
		t.Fatal("existing stakeholder was changed")
	}
	if f.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='relationship'`) != audits {
		t.Fatal("reused stakeholder was written again")
	}
	f.closeDeal(t)
	var hasDeal *contacts.ContactHasDealError
	if _, err := f.contacts.DemoteLead(ctx, f.lead, "Wrong qualification"); !errors.As(err, &hasDeal) {
		t.Fatalf("undo of merged contact: %v", err)
	}
	if f.e.WsCount(t, `SELECT count(*) FROM deal WHERE id=$1 AND archived_at IS NULL`, dealID) != 1 || f.e.WsCount(t, `SELECT count(*) FROM relationship WHERE contact_id=$1 AND deal_id=$2 AND archived_at IS NULL`, contactID, dealID) != 1 {
		t.Fatal("undo changed the existing deal or stakeholder")
	}
}

func (f existingQualification) closeDeal(t *testing.T) {
	t.Helper()
	for _, stage := range *f.pipeline.Stages {
		if stage.Semantic != "lost" {
			continue
		}
		if _, err := f.deals.AdvanceDeal(f.e.Admin(), ids.From[ids.DealKind](ids.UUID(f.deal.Id)), deals.AdvanceDealInput{ToStageID: ids.From[ids.StageKind](ids.UUID(stage.Id)), LostReason: new("No renewal")}); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Fatal("default pipeline has no lost stage")
}

func TestQualificationPreviewContinuesEligibleDealsAcrossPages(t *testing.T) {
	f := qualificationWithExistingDeal(t)
	for range 25 {
		if _, err := f.deals.CreateDeal(f.e.Admin(), deals.CreateDealInput{Name: "Another renewal", CompanyID: &f.company, PipelineID: ids.From[ids.PipelineKind](ids.UUID(f.pipeline.Id)), StageID: ids.From[ids.StageKind](ids.UUID((*f.pipeline.Stages)[0].Id)), Source: "manual"}); err != nil {
			t.Fatal(err)
		}
	}
	first, err := f.contacts.PreviewLeadPromotion(f.e.Admin(), f.lead)
	if err != nil || first.ExistingDeals == nil || len(*first.ExistingDeals) != 25 || first.ExistingDealsCursor == nil {
		t.Fatalf("first page: %+v, %v", first, err)
	}
	next, err := f.contacts.PreviewLeadPromotionWithOptions(f.e.Admin(), f.lead, contacts.PromotionPreviewOptions{DealsCursor: first.ExistingDealsCursor})
	if err != nil || next.ExistingDeals == nil || len(*next.ExistingDeals) != 1 || next.ExistingDealsCursor != nil {
		t.Fatalf("next page: %+v, %v", next, err)
	}
	for _, deal := range *first.ExistingDeals {
		if deal.Id == (*next.ExistingDeals)[0].Id {
			t.Fatal("the next page repeats a deal")
		}
	}
}

func TestQualificationFiltersReadOnlySharesBeforePagingAndRechecksAtWrite(t *testing.T) {
	f := qualificationWithExistingDeal(t)
	rep := ids.From[ids.UserKind](f.e.Rep1)
	if _, err := f.contacts.UpdateCompany(f.e.Admin(), f.company, contacts.UpdateCompanyInput{OwnerID: &rep}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.contacts.UpdateLead(f.e.Admin(), f.lead, contacts.UpdateLeadInput{OwnerID: &rep}); err != nil {
		t.Fatal(err)
	}
	shares := identity.NewServiceFor(f.e.DB())
	grant := identity.CreateGrantInput{RecordType: "deal", RecordID: ids.UUID(f.deal.Id), SubjectType: "user", SubjectID: f.e.Rep1, Access: "read"}
	if _, err := shares.CreateRecordGrant(f.e.Admin(), grant); err != nil {
		t.Fatal(err)
	}
	perms := integration.AdminPerms
	perms.RowScope, perms.RoleKeys = principal.RowScopeOwn, []string{"rep"}
	ctx := f.e.As(f.e.Rep1, nil, perms)
	preview, err := f.contacts.PreviewLeadPromotion(ctx, f.lead)
	if err != nil || (preview.ExistingDeals != nil && len(*preview.ExistingDeals) != 0) {
		t.Fatalf("read-only deal leaked into picker: %+v, %v", preview, err)
	}
	id := ids.UUID(f.deal.Id)
	if _, err := f.contacts.QualifyLead(ctx, f.lead, contacts.PromoteLeadInput{Trigger: "human_qualify", ExistingDealID: &id}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("read share admitted qualification: %v", err)
	}
	if f.e.WsCount(t, `SELECT count(*) FROM contact WHERE converted_from_lead_id=$1`, f.lead) != 0 {
		t.Fatal("read-share refusal left a contact")
	}
	grant.Access = "write"
	if _, err := shares.CreateRecordGrant(f.e.Admin(), grant); err != nil {
		t.Fatal(err)
	}
	preview, err = f.contacts.PreviewLeadPromotion(ctx, f.lead)
	if err != nil || preview.ExistingDeals == nil || len(*preview.ExistingDeals) != 1 {
		t.Fatalf("write share omitted from picker: %+v, %v", preview, err)
	}
	if _, err := f.contacts.QualifyLead(ctx, f.lead, contacts.PromoteLeadInput{Trigger: "human_qualify", ExistingDealID: &id}); err != nil {
		t.Fatalf("write share refused: %v", err)
	}
}
