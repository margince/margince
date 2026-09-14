// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The two counts every company row carries (PO-EXT-10, AC-companies-2/3),
// proven over a real migrated Postgres: contact_count follows the
// current-primary employment edges the real writer makes, open_deal_count
// follows the 0065 view, the list and the single read agree, and a role
// without computed_field:read is shown a contact count but no deal count.

import (
	"encoding/json"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestCompanyCounts_ListAndSingleReadAgreeWithTheEdges(t *testing.T) {
	e := Setup(t)
	acme := e.SeedCompany(t, "Acme Counts", nil)
	quiet := e.SeedCompany(t, "Quiet Counts", nil)
	staff := e.SeedContact(t, "Works At Acme", nil)
	second := e.SeedContact(t, "Also At Acme", nil)
	leaver := e.SeedContact(t, "Left Acme", nil)

	employ := func(contact, company ids.UUID, ended *time.Time) {
		t.Helper()
		contactID := ids.From[ids.ContactKind](contact)
		companyID := ids.From[ids.CompanyKind](company)
		if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
			Kind: "employment", ContactID: &contactID, CompanyID: &companyID,
			IsCurrentPrimary: boolPtr(ended == nil), EndedAt: ended, Source: "manual",
		}); err != nil {
			t.Fatalf("seeding the employment edge: %v", err)
		}
	}
	left := time.Date(2021, 6, 30, 0, 0, 0, 0, time.UTC)
	employ(staff, acme, nil)
	employ(second, acme, nil)
	// A past employer is not a contact: the column answers who works here. Dated
	// as over, because that is what past is — an undated non-primary job cannot
	// say it, now that a contact's only employment is their current primary one.
	employ(leaver, acme, &left)

	pipeline, open := pipelineFixtureFor(e.Admin(), t, e.Deals)
	for _, name := range []string{"D1", "D2", "D3"} {
		if _, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
			Name: name, AmountMinor: int64Ptr(1000), Currency: strPtr("EUR"),
			PipelineID: pipeline, StageID: open, CompanyID: companyIDPtr(companyIDOf(acme)), Source: "manual",
		}); err != nil {
			t.Fatal(err)
		}
	}

	byID := func(rows []crmcontracts.Company, id ids.UUID) crmcontracts.Company {
		t.Helper()
		for _, o := range rows {
			if ids.UUID(o.Id) == id {
				return o
			}
		}
		t.Fatalf("company %s missing from the page", id)
		return crmcontracts.Company{}
	}
	page, _, err := e.Contacts.ListCompanies(e.Admin(), contacts.ListCompaniesInput{})
	if err != nil {
		t.Fatal(err)
	}
	assertCounts(t, "list acme", byID(page, acme), 2, 3)
	// Zero is a number here, not an absence: a reader must be able to tell
	// "no contacts" from "not shown".
	assertCounts(t, "list quiet", byID(page, quiet), 0, 0)

	single, err := e.Contacts.GetCompany(e.Admin(), companyIDOf(acme), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	assertCounts(t, "single acme", single, 2, 3)
}

// A role without computed_field:read is STATE-4 for the deal count: the key
// is absent from the wire, not zero. The contact count is an edge fact and
// stays.
func TestCompanyCounts_UngatedRoleSeesContactsButNoDealCount(t *testing.T) {
	e := Setup(t)
	acme := e.SeedCompany(t, "Gated Counts", nil)
	staff := e.SeedContact(t, "Works At Gated", nil)
	pID := ids.From[ids.ContactKind](staff)
	oID := ids.From[ids.CompanyKind](acme)
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &pID, CompanyID: &oID, IsCurrentPrimary: boolPtr(true), Source: "manual",
	}); err != nil {
		t.Fatal(err)
	}
	// contact:read, deal:read and relationship:read granted, computed_field:read
	// not: this is the STATE-4 case for the deal count ALONE, so every grant the
	// contact count needs is present — the edge one included, since the count is
	// over employment pairs.
	perms := principal.Permissions{
		RoleKeys: computedFieldNoGrantPerms.RoleKeys,
		Objects: map[string]principal.ObjectGrant{
			"company": {Read: true}, "contact": {Read: true}, "deal": {Read: true},
			"relationship":          {Read: true},
			"installation_settings": {Read: true},
		},
		RowScope: principal.RowScopeAll,
	}
	ctx := e.As(e.Rep1, nil, perms)

	page, _, err := e.Contacts.ListCompanies(ctx, contacts.ListCompaniesInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page) == 0 {
		t.Fatal("the ungated role must still list companies")
	}
	var got *crmcontracts.Company
	for i := range page {
		if ids.UUID(page[i].Id) == acme {
			got = &page[i]
		}
	}
	if got == nil {
		t.Fatal("seeded company missing from the ungated page")
	}
	if got.ContactCount == nil || *got.ContactCount != 1 {
		t.Fatalf("contact_count = %v, want 1 for the ungated role", got.ContactCount)
	}
	if got.OpenDealCount != nil {
		t.Fatalf("open_deal_count = %d, want it withheld for a role without computed_field:read", *got.OpenDealCount)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if _, present := wire["open_deal_count"]; present {
		t.Fatalf("want the open_deal_count KEY absent from the wire, got %v", wire["open_deal_count"])
	}
	if _, present := wire["contact_count"]; !present {
		t.Fatal("want contact_count on the wire for the ungated role")
	}
}

// The contact count is a read under the caller's contact row scope: a number
// that moved when a colleague captured a private contact would disclose that
// contact. The deal count is the account's whole open pipeline, as the
// company page's tile sums it (PO-EXT-10, founder decision 2026-08-18).
func TestCompanyCounts_FollowTheCallersRowScope(t *testing.T) {
	e := Setup(t)
	// The account is unowned, so it is visible at every scope tier.
	acme := e.SeedCompany(t, "Shared Counts", nil)
	mine := e.SeedContact(t, "Rep1 Contact", &e.Rep1)
	theirs := e.SeedContact(t, "Rep3 Contact", &e.Rep3)
	employ := func(contact ids.UUID) {
		t.Helper()
		contactID := ids.From[ids.ContactKind](contact)
		companyID := ids.From[ids.CompanyKind](acme)
		if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
			Kind: "employment", ContactID: &contactID, CompanyID: &companyID,
			IsCurrentPrimary: boolPtr(true), Source: "manual",
		}); err != nil {
			t.Fatalf("seeding the employment edge: %v", err)
		}
	}
	employ(mine)
	employ(theirs)
	// Ownership alone no longer narrows a contact; capture privacy does.
	// Applied after the edge is seeded, since the seeding admin is not the
	// captor and could not link to a private contact.
	e.MakeCapturePrivate(t, "contact", theirs, e.Rep3)

	pipeline, open := pipelineFixtureFor(e.Admin(), t, e.Deals)
	for _, owner := range []ids.UUID{e.Rep1, e.Rep3} {
		ownerID := ids.From[ids.UserKind](owner)
		if _, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
			Name: "Deal of " + owner.String(), AmountMinor: int64Ptr(1000), Currency: strPtr("EUR"),
			PipelineID: pipeline, StageID: open, CompanyID: companyIDPtr(companyIDOf(acme)),
			OwnerID: &ownerID, Source: "manual",
		}); err != nil {
			t.Fatal(err)
		}
	}

	// The captor reads both contacts: their own private one and the shared one.
	captor, err := e.Contacts.GetCompany(e.As(e.Rep3, []ids.UUID{e.Team2}, AdminPerms), companyIDOf(acme), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	assertCounts(t, "the captor sees both contacts", captor, 2, 2)

	perms := rollupCompanyReadPerms(principal.RowScopeOwn)
	perms.Objects["computed_field"] = principal.ObjectGrant{Read: true}
	perms.Objects["contact"] = principal.ObjectGrant{Read: true}
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)
	own, err := e.Contacts.GetCompany(rep, companyIDOf(acme), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	assertCounts(t, "own-scope rep: readable contacts, whole pipeline", own, 1, 2)
	page, _, err := e.Contacts.ListCompanies(rep, contacts.ListCompaniesInput{})
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range page {
		if ids.UUID(o.Id) == acme {
			assertCounts(t, "own-scope rep on the list", o, 1, 2)
			return
		}
	}
	t.Fatal("shared account missing from the rep's page")
}

// The object grant comes first: no deal:read, no deal count — even with
// computed_field:read — and no contact:read, no contact count. Absent, not 0.
func TestCompanyCounts_ObjectGrantGatesEachCount(t *testing.T) {
	e := Setup(t)
	acme := e.SeedCompany(t, "Grant Gated", nil)
	perms := rollupCompanyReadPerms(principal.RowScopeAll)
	perms.Objects["computed_field"] = principal.ObjectGrant{Read: true}
	// deal:read is in rollupCompanyReadPerms; contact:read is not.
	delete(perms.Objects, "deal")
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)

	got, err := e.Contacts.GetCompany(ctx, companyIDOf(acme), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got.OpenDealCount != nil {
		t.Fatalf("open_deal_count = %d, want it withheld without deal:read", *got.OpenDealCount)
	}
	if got.ContactCount != nil {
		t.Fatalf("contact_count = %d, want it withheld without contact:read", *got.ContactCount)
	}
}

func assertCounts(t *testing.T, label string, o crmcontracts.Company, contacts, deals int) {
	t.Helper()
	if o.ContactCount == nil {
		t.Fatalf("%s: contact_count absent, want %d", label, contacts)
	}
	if *o.ContactCount != contacts {
		t.Fatalf("%s: contact_count = %d, want %d", label, *o.ContactCount, contacts)
	}
	if o.OpenDealCount == nil {
		t.Fatalf("%s: open_deal_count absent, want %d", label, deals)
	}
	if *o.OpenDealCount != deals {
		t.Fatalf("%s: open_deal_count = %d, want %d", label, *o.OpenDealCount, deals)
	}
}
