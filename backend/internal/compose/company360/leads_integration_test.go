// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package company360_test

import (
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestCompanyLeadsUseConfirmedLinksAndKeepTheReadersScope(t *testing.T) {
	e := integration.Setup(t)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Company from leads", &e.Rep1))
	other := ids.From[ids.CompanyKind](e.SeedCompany(t, "Another company", &e.Rep1))
	var linked []ids.UUID
	for i := range 7 {
		owner := ids.From[ids.UserKind](e.Rep1)
		if i == 6 {
			owner = ids.From[ids.UserKind](e.Rep2)
		}
		lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: new("Linked buyer"), CompanyName: new("Company from leads"), Source: "manual", OwnerID: &owner})
		if err != nil {
			t.Fatal(err)
		}
		id := ids.From[ids.LeadKind](ids.UUID(lead.Id))
		if _, err := e.Contacts.LinkLeadCompany(e.Admin(), id, company); err != nil {
			t.Fatal(err)
		}
		linked = append(linked, id.UUID)
	}
	unlinked, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: new("Only a name match"), CompanyName: new("Company from leads"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	outside, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: new("Another company buyer"), CompanyName: new("Another company"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Contacts.LinkLeadCompany(e.Admin(), ids.From[ids.LeadKind](ids.UUID(outside.Id)), other); err != nil {
		t.Fatal(err)
	}
	cursor := new("")
	var seen []ids.UUID
	for range 10 {
		data, page, err := e.Contacts.ListLeads(e.Admin(), contacts.ListLeadsInput{CompanyID: &company, Limit: new(2), Cursor: cursor, Sort: new("-created_at")})
		if err != nil {
			t.Fatal(err)
		}
		if len(data) > 2 {
			t.Fatalf("page has %d leads", len(data))
		}
		for _, lead := range data {
			if lead.CompanyId == nil || ids.UUID(*lead.CompanyId) != company.UUID || ids.UUID(lead.Id) == ids.UUID(unlinked.Id) || ids.UUID(lead.Id) == ids.UUID(outside.Id) {
				t.Fatalf("unrelated lead leaked: %+v", lead)
			}
			if slices.Contains(seen, ids.UUID(lead.Id)) {
				t.Fatal("paging repeated a lead")
			}
			seen = append(seen, ids.UUID(lead.Id))
		}
		if !page.HasMore {
			break
		}
		*cursor = page.NextCursor
	}
	if len(seen) != 7 {
		t.Fatalf("company list has %d linked leads", len(seen))
	}
	data, _, err := e.Contacts.ListLeads(e.As(e.Rep1, nil, principal.Permissions{Objects: map[string]principal.ObjectGrant{"company": {Read: true}, "lead": {Read: true}}, RowScope: principal.RowScopeOwn}), contacts.ListLeadsInput{CompanyID: &company, Limit: new(100)})
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 6 {
		t.Fatalf("own scope has %d leads, want 6", len(data))
	}
	now := clocktest.Now(t)
	service := company360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects, approvals.NewService(e.DB()), func() time.Time { return now })
	view, err := service.Assemble(e.Admin(), company)
	if err != nil {
		t.Fatal(err)
	}
	if view.Leads == nil || len(view.Leads.Data) != 5 || !view.Leads.Page.HasMore || view.Leads.Page.NextCursor == nil {
		t.Fatalf("company leads page=%+v", view.Leads)
	}
	limited := principal.WithActor(e.Admin(), principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + e.AdminUser.String(), UserID: e.AdminUser, Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"company": {Read: true}}, RowScope: principal.RowScopeAll}})
	hidden, err := service.Assemble(limited, company)
	if err != nil {
		t.Fatal(err)
	}
	if hidden.Leads != nil || !slices.Contains(hidden.SectionsOmitted, "leads") {
		t.Fatalf("missing lead grant drew an empty section: %+v", hidden.Leads)
	}
}
