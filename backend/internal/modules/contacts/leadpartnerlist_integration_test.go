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

	noPartnerRead := asStageAdmin(e, map[string]principal.ObjectGrant{"lead": {Read: true}, "company": {Read: true}})
	for _, filter := range []LeadPartnerFilter{{CompanyID: &first}, {Accepted: &waiting}} {
		if _, _, err := e.store.ListLeads(noPartnerRead, ListLeadsInput{Partner: filter}); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("a seat without partner read filtering by %+v: err = %v, want permission denied", filter, err)
		}
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
	if got := listedLeadNames(blind, t, e, LeadPartnerFilter{CompanyID: &partnerID}); len(got) != 0 {
		t.Errorf("a seat that cannot open the partner found its referrals by id: %v", got)
	}
	waiting := false
	if got := listedLeadNames(blind, t, e, LeadPartnerFilter{Accepted: &waiting}); !slices.Equal(got, []string{name}) {
		t.Errorf("the seat's own referred lead, awaiting acceptance: listed %v, want [%s]", got, name)
	}
	if got := listedLeadNames(admin, t, e, LeadPartnerFilter{CompanyID: &partnerID}); !slices.Equal(got, []string{name}) {
		t.Errorf("the partner company's owner: listed %v, want [%s]", got, name)
	}
}
