// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A lead names its referring partner when it is created. The lead list
// narrows to a partner's referrals, or to the ones awaiting acceptance.

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// listedLeadNames lists leads through the store's list read and answers the
// names on the page, sorted.
func listedLeadNames(ctx context.Context, t *testing.T, e *dedupeEnv, partner LeadPartnerFilter) []string {
	t.Helper()
	leads, _, err := e.store.ListLeads(ctx, ListLeadsInput{Partner: partner})
	if err != nil {
		t.Fatalf("listing the leads by %+v: %v", partner, err)
	}
	names := make([]string, 0, len(leads))
	for _, l := range leads {
		if l.FullName != nil {
			names = append(names, *l.FullName)
		}
	}
	slices.Sort(names)
	return names
}

func TestALeadNamesItsReferringPartnerWhenCreated(t *testing.T) {
	e := setupDedupe(t)
	ctx := asStageAdmin(e, stageAdminGrants())
	partnerID, _ := newPartnerCompany(ctx, t, e, "Referrer GmbH", UpsertPartnerInput{PartnerRole: "referral"})

	name := "Referred At Birth"
	lead, _, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: &name, Source: "manual", PartnerCompanyID: &partnerID})
	if err != nil {
		t.Fatalf("creating a referred lead: %v", err)
	}
	if lead.PartnerCompanyId == nil || ids.UUID(*lead.PartnerCompanyId) != partnerID.UUID {
		t.Errorf("the created lead names partner %v, want %s", lead.PartnerCompanyId, partnerID)
	}

	plain, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Not A Partner AG", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	notPartner := ids.From[ids.CompanyKind](ids.UUID(plain.Id))
	refused := "Referred By Nobody"
	_, _, err = e.store.CreateLead(ctx, CreateLeadInput{FullName: &refused, Source: "manual", PartnerCompanyID: &notPartner})
	if fieldCode(err) != "not_a_partner" {
		t.Errorf("creating a lead that names a company with no programme: err = %v, want not_a_partner", err)
	}
	if got := listedLeadNames(ctx, t, e, LeadPartnerFilter{}); slices.Contains(got, refused) {
		t.Errorf("the refused lead was written anyway: %v", got)
	}
}

func TestTheLeadListNarrowsToAPartnersReferrals(t *testing.T) {
	e := setupDedupe(t)
	ctx := asStageAdmin(e, stageAdminGrants())
	first, _ := newPartnerCompany(ctx, t, e, "First Partner AG", UpsertPartnerInput{PartnerRole: "referral", RelationshipStage: new("ready")})
	second, _ := newPartnerCompany(ctx, t, e, "Second Partner AG", UpsertPartnerInput{PartnerRole: "referral"})
	acceptReferredLead(ctx, t, e, first, "Accepted Referral")
	referredLead(ctx, t, e, first, "Waiting Referral")
	referredLead(ctx, t, e, second, "Other Partner's Referral")
	unreferred := "Walk-in"
	if _, _, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: &unreferred, Source: "manual"}); err != nil {
		t.Fatal(err)
	}

	accepted, waiting := true, false
	for _, tc := range []struct {
		name   string
		filter LeadPartnerFilter
		want   []string
	}{
		{"one partner's referrals", LeadPartnerFilter{CompanyID: &first}, []string{"Accepted Referral", "Waiting Referral"}},
		{"the accepted referrals", LeadPartnerFilter{Accepted: &accepted}, []string{"Accepted Referral"}},
		{"the referrals awaiting acceptance", LeadPartnerFilter{Accepted: &waiting}, []string{"Other Partner's Referral", "Waiting Referral"}},
		{"one partner's referrals awaiting acceptance", LeadPartnerFilter{CompanyID: &first, Accepted: &waiting}, []string{"Waiting Referral"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := listedLeadNames(ctx, t, e, tc.filter); !slices.Equal(got, tc.want) {
				t.Errorf("listed %v, want %v", got, tc.want)
			}
		})
	}
}

// The lead's own read blanks a partner the caller cannot open. Filtering by
// that partner's id must not give back the link the read withholds.
func TestALeadListFilterByAnUnseenPartnerMatchesNothing(t *testing.T) {
	e := setupDedupe(t)
	admin := asStageAdmin(e, stageAdminGrants())
	owned, err := e.store.CreateCompany(admin, CreateCompanyInput{
		DisplayName: "Private Partner KG", Source: "manual", OwnerID: new(ids.From[ids.UserKind](e.rep)),
	})
	if err != nil {
		t.Fatal(err)
	}
	partnerID := ids.From[ids.CompanyKind](ids.UUID(owned.Id))
	if _, err := e.store.UpsertPartner(admin, UpsertPartnerInput{CompanyID: partnerID, PartnerRole: "referral"}); err != nil {
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
	name := "Colleague's Referral"
	own, _, err := e.store.CreateLead(colleague(principal.RowScopeAll), CreateLeadInput{
		FullName: &name, Source: "manual", OwnerID: new(ids.From[ids.UserKind](e.otherRep)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.UpdateLead(admin, ids.From[ids.LeadKind](ids.UUID(own.Id)), UpdateLeadInput{PartnerCompanyID: &partnerID}); err != nil {
		t.Fatalf("naming the partner: %v", err)
	}

	blind := colleague(principal.RowScopeOwn)
	accepted, waiting := true, false
	if got := listedLeadNames(blind, t, e, LeadPartnerFilter{CompanyID: &partnerID}); len(got) != 0 {
		t.Errorf("a seat that cannot open the partner found its referrals by id: %v", got)
	}
	if got := listedLeadNames(blind, t, e, LeadPartnerFilter{Accepted: &waiting}); len(got) != 0 {
		t.Errorf("a seat that cannot open the partner found the lead awaiting it: %v", got)
	}
	if got := listedLeadNames(admin, t, e, LeadPartnerFilter{CompanyID: &partnerID}); !slices.Equal(got, []string{name}) {
		t.Errorf("the partner company's owner: listed %v, want [%s]", got, name)
	}

	leadID := ids.From[ids.LeadKind](ids.UUID(own.Id))
	if _, err := e.store.AcceptPartnerLead(admin, leadID); err != nil {
		t.Fatalf("accepting the lead: %v", err)
	}
	if got := listedLeadNames(blind, t, e, LeadPartnerFilter{Accepted: &accepted}); len(got) != 0 {
		t.Errorf("a seat that cannot open the partner found the accepted lead: %v", got)
	}
	read, err := e.store.GetLead(blind, leadID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if read.PartnerCompanyId != nil || read.PartnerAcceptedAt != nil || read.PartnerAcceptedBy != nil {
		t.Errorf("the lead's read gave a seat that cannot open the partner partner %v, accepted %v by %v",
			read.PartnerCompanyId, read.PartnerAcceptedAt, read.PartnerAcceptedBy)
	}
	// The blank read must not reopen the link: the accepted partner stays.
	other, _ := newPartnerCompany(admin, t, e, "Visible Partner AG", UpsertPartnerInput{PartnerRole: "referral"})
	if _, err := e.store.UpdateLead(blind, leadID, UpdateLeadInput{PartnerCompanyID: &other}); messageCode(err) != "partner_already_accepted" {
		t.Errorf("replacing an accepted partner the seat cannot see: err = %v, want partner_already_accepted", err)
	}
}

// Whether a company is a partner is partner data. A seat without partner read
// cannot name one on a lead, so the refusal does not answer that question.
func TestNamingALeadsPartnerTakesPartnerRead(t *testing.T) {
	e := setupDedupe(t)
	admin := asStageAdmin(e, stageAdminGrants())
	partnerID, _ := newPartnerCompany(admin, t, e, "Referrer GmbH", UpsertPartnerInput{PartnerRole: "referral"})
	noPartnerRead := asStageAdmin(e, map[string]principal.ObjectGrant{
		"lead": {Create: true, Read: true, Update: true}, "company": {Read: true},
	})

	name := "Referred Without The Grant"
	_, _, err := e.store.CreateLead(noPartnerRead, CreateLeadInput{FullName: &name, Source: "manual", PartnerCompanyID: &partnerID})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("creating a referred lead without partner read: err = %v, want permission denied", err)
	}
	plain := "Unreferred"
	lead, _, err := e.store.CreateLead(noPartnerRead, CreateLeadInput{FullName: &plain, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.store.UpdateLead(noPartnerRead, ids.From[ids.LeadKind](ids.UUID(lead.Id)), UpdateLeadInput{PartnerCompanyID: &partnerID})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("naming the partner on an update without partner read: err = %v, want permission denied", err)
	}
}
