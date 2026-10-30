// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A lead a partner referred, and the acceptance that activates the partner:
// the stamp, the move to the first active stage, the refusals, and who may.

import (
	"context"
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// referredLead creates a lead and names the partner company that referred it,
// through the lead's own writers.
func referredLead(ctx context.Context, t *testing.T, e *dedupeEnv, partner ids.CompanyID, name string) ids.LeadID {
	t.Helper()
	lead, _, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: &name, Source: "manual"})
	if err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	id := ids.From[ids.LeadKind](ids.UUID(lead.Id))
	if _, err := e.store.UpdateLead(ctx, id, UpdateLeadInput{PartnerCompanyID: &partner}); err != nil {
		t.Fatalf("naming the partner of %s: %v", name, err)
	}
	return id
}

func acceptReferredLead(ctx context.Context, t *testing.T, e *dedupeEnv, partner ids.CompanyID, name string) crmcontracts.Lead {
	t.Helper()
	accepted, err := e.store.AcceptPartnerLead(ctx, referredLead(ctx, t, e, partner, name))
	if err != nil {
		t.Fatalf("accepting %s: %v", name, err)
	}
	return accepted
}

func messageCode(err error) string {
	var fault interface{ MessageFault() (string, string) }
	if errors.As(err, &fault) {
		code, _ := fault.MessageFault()
		return code
	}
	return fieldCode(err)
}

func TestAcceptingAPartnersFirstLeadActivatesIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := asStageAdmin(e, stageAdminGrants())
	partnerID, _ := newPartnerCompany(ctx, t, e, "Referrer GmbH", UpsertPartnerInput{PartnerRole: "consulting", RelationshipStage: new("ready")})

	first := acceptReferredLead(ctx, t, e, partnerID, "First Referral")
	if first.PartnerAcceptedAt == nil || first.PartnerAcceptedBy == nil {
		t.Fatalf("the accepted lead carries no stamp: %+v", first)
	}
	partner, err := e.store.GetPartner(ctx, partnerID)
	if err != nil {
		t.Fatal(err)
	}
	if partner.ActivationBasis == nil || *partner.ActivationBasis != "lead_accepted" ||
		partner.ActivationLeadId == nil || *partner.ActivationLeadId != first.Id || *partner.RelationshipStage != "activated" {
		t.Fatalf("after the first acceptance the partner reads basis %v, lead %v, stage %s; want lead_accepted through it, on activated",
			partner.ActivationBasis, partner.ActivationLeadId, *partner.RelationshipStage)
	}

	if _, err := e.store.AcceptPartnerLead(ctx, ids.From[ids.LeadKind](ids.UUID(first.Id))); messageCode(err) != "already_accepted" {
		t.Errorf("a second acceptance of one lead: err = %v, want already_accepted", err)
	}
	acceptReferredLead(ctx, t, e, partnerID, "Second Referral")
	again, err := e.store.GetPartner(ctx, partnerID)
	if err != nil {
		t.Fatal(err)
	}
	if *again.ActivationLeadId != first.Id || !again.ActivatedAt.Equal(*partner.ActivatedAt) {
		t.Errorf("a second accepted lead moved the activation stamp to %v", again.ActivationLeadId)
	}
	_, err = e.store.UpdateLead(ctx, ids.From[ids.LeadKind](ids.UUID(first.Id)), UpdateLeadInput{Clear: []string{leadPartnerField}})
	if messageCode(err) != "partner_already_accepted" {
		t.Errorf("clearing the partner of an accepted lead: err = %v, want partner_already_accepted", err)
	}
}

func TestALeadIsAcceptedOnlyForALivePartner(t *testing.T) {
	e := setupDedupe(t)
	ctx := asStageAdmin(e, stageAdminGrants())
	name := "Unreferred"
	lead, _, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: &name, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	leadID := ids.From[ids.LeadKind](ids.UUID(lead.Id))
	if _, err := e.store.AcceptPartnerLead(ctx, leadID); fieldCode(err) != "no_partner" {
		t.Errorf("accepting a lead with no partner: err = %v, want no_partner", err)
	}
	plain, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Not A Partner AG", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	notPartner := ids.From[ids.CompanyKind](ids.UUID(plain.Id))
	if _, err := e.store.UpdateLead(ctx, leadID, UpdateLeadInput{PartnerCompanyID: &notPartner}); fieldCode(err) != "not_a_partner" {
		t.Errorf("naming a company with no programme: err = %v, want not_a_partner", err)
	}

	partnerID, _ := newPartnerCompany(ctx, t, e, "Live Partner AG", UpsertPartnerInput{PartnerRole: "referral"})
	dropped := referredLead(ctx, t, e, partnerID, "Dropped Referral")
	closer := asStageAdmin(e, map[string]principal.ObjectGrant{
		"lead": {Read: true, Update: true, Delete: true}, "partner": {Read: true, Update: true}, "company": {Read: true},
	})
	if _, err := e.store.DisqualifyLead(closer, dropped, DisqualifyLeadInput{}); err != nil {
		t.Fatalf("disqualifying the lead: %v", err)
	}
	if _, err := e.store.AcceptPartnerLead(closer, dropped); fieldCode(err) != "lead_disqualified" {
		t.Errorf("accepting a disqualified lead: err = %v, want lead_disqualified", err)
	}
}

func TestOnlyAPartnerManagerAcceptsAPartnerLead(t *testing.T) {
	e := setupDedupe(t)
	admin := asStageAdmin(e, stageAdminGrants())
	// Private to its owner, so a colleague cannot see it.
	owned, err := e.store.CreateCompany(admin, CreateCompanyInput{
		DisplayName: "Guarded Partner KG", Source: "manual", OwnerID: new(ids.From[ids.UserKind](e.rep)),
	})
	if err != nil {
		t.Fatal(err)
	}
	partnerID := ids.From[ids.CompanyKind](ids.UUID(owned.Id))
	if _, err := e.store.UpsertPartner(admin, UpsertPartnerInput{CompanyID: partnerID, PartnerRole: "hosting"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.UpdateCompany(admin, partnerID, UpdateCompanyInput{Visibility: new("owner")}); err != nil {
		t.Fatalf("making the partner company private: %v", err)
	}
	colleague := func(scope principal.RowScope) context.Context {
		return principal.WithActor(admin, principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:" + e.otherRep.String(), UserID: e.otherRep,
			Permissions: principal.Permissions{Objects: stageAdminGrants(), RowScope: scope},
		})
	}
	// The colleague's own lead, which the partner company's owner then names
	// the partner on: the colleague may write the lead and not see the partner.
	name := "Guarded Referral"
	own, _, err := e.store.CreateLead(colleague(principal.RowScopeAll), CreateLeadInput{FullName: &name, Source: "manual", OwnerID: new(ids.From[ids.UserKind](e.otherRep))})
	if err != nil {
		t.Fatal(err)
	}
	leadID := ids.From[ids.LeadKind](ids.UUID(own.Id))
	if _, err := e.store.UpdateLead(admin, leadID, UpdateLeadInput{PartnerCompanyID: &partnerID}); err != nil {
		t.Fatalf("naming the partner: %v", err)
	}

	rep := asStageAdmin(e, map[string]principal.ObjectGrant{
		"company": {Read: true, Update: true}, "lead": {Read: true, Update: true}, "partner": {Read: true},
	})
	if _, err := e.store.AcceptPartnerLead(rep, leadID); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a seat without the partner update grant: err = %v, want permission denied", err)
	}
	agent := principal.WithActor(admin, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:test", UserID: e.rep,
		Permissions: principal.Permissions{Objects: stageAdminGrants(), RowScope: principal.RowScopeAll},
	})
	if _, err := e.store.AcceptPartnerLead(agent, leadID); err == nil {
		t.Error("an agent accepted a partner lead")
	}
	if _, err := e.store.AcceptPartnerLead(colleague(principal.RowScopeOwn), leadID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a seat that cannot see the partner company: err = %v, want not found", err)
	}
}

func TestASeatThatCannotWriteThePartnerCompanyAcceptsNothing(t *testing.T) {
	e := setupDedupe(t)
	admin := asStageAdmin(e, stageAdminGrants())
	// Owned by the admin and readable by everyone: the colleague sees it and,
	// confined to their own rows, may not write it.
	owned, err := e.store.CreateCompany(admin, CreateCompanyInput{
		DisplayName: "Read Only Partner AG", Source: "manual", OwnerID: new(ids.From[ids.UserKind](e.rep)),
	})
	if err != nil {
		t.Fatal(err)
	}
	partnerID := ids.From[ids.CompanyKind](ids.UUID(owned.Id))
	if _, err := e.store.UpsertPartner(admin, UpsertPartnerInput{CompanyID: partnerID, PartnerRole: "referral"}); err != nil {
		t.Fatal(err)
	}
	colleague := func(scope principal.RowScope) context.Context {
		return principal.WithActor(admin, principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:" + e.otherRep.String(), UserID: e.otherRep,
			Permissions: principal.Permissions{Objects: stageAdminGrants(), RowScope: scope},
		})
	}
	name := "Colleague's Referral"
	own, _, err := e.store.CreateLead(colleague(principal.RowScopeAll), CreateLeadInput{
		FullName: &name, Source: "manual", OwnerID: new(ids.From[ids.UserKind](e.otherRep)),
	})
	if err != nil {
		t.Fatal(err)
	}
	leadID := ids.From[ids.LeadKind](ids.UUID(own.Id))
	if _, err := e.store.UpdateLead(admin, leadID, UpdateLeadInput{PartnerCompanyID: &partnerID}); err != nil {
		t.Fatalf("naming the partner: %v", err)
	}

	if _, err := e.store.AcceptPartnerLead(colleague(principal.RowScopeOwn), leadID); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a seat that may not write the partner company: err = %v, want permission denied", err)
	}
	lead, err := e.store.GetLead(admin, leadID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	partner, err := e.store.GetPartner(admin, partnerID)
	if err != nil {
		t.Fatal(err)
	}
	if lead.PartnerAcceptedAt != nil || partner.ActivationBasis != nil {
		t.Errorf("a refused acceptance left the lead stamped %v and the partner activated %v", lead.PartnerAcceptedAt, partner.ActivationBasis)
	}
}
