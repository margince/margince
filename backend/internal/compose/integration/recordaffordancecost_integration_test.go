// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestRecordAffordancesShareAuthorityOnStoreReads(t *testing.T) {
	e := Setup(t)
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	e.Contacts.SetClock(func() time.Time { return now })
	e.Deals.SetClock(func() time.Time { return now })
	pipeline, open, _ := DealFixture(t, e)
	contact := ids.From[ids.ContactKind](e.SeedContact(t, "Authority contact", &e.Rep1))
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Authority company", &e.Rep1))
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Authority deal", pipeline, open, &e.Rep1))
	lead := seedLead(t, e, "Authority lead", "authority-lead@prospect.test", &e.Rep1)
	// A live project blocks its company's archive, so it sits on another company.
	project := seedProject(e.Admin(), t, e, "Authority project", e.SeedCompany(t, "Project customer", nil), &e.Rep1).ID
	// want holds the absolute statement counts for delete-only and update+delete.
	// A second authority read on either path changes them.
	cases := []struct {
		name string
		want [2]int64
		read func(context.Context, bool)
	}{
		{"contact", recordReadCost.contact, func(ctx context.Context, writable bool) {
			row, err := e.Contacts.GetContact(ctx, contact, storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			requireRecordAffordances(t, row.Writable, row.Archivable, expectedRecordActions{writable: writable, archivable: true})
		}},
		{"company", recordReadCost.company, func(ctx context.Context, writable bool) {
			row, err := e.Contacts.GetCompany(ctx, company, storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			requireRecordAffordances(t, row.Writable, row.Archivable, expectedRecordActions{writable: writable, archivable: true})
		}},
		{"deal", recordReadCost.deal, func(ctx context.Context, writable bool) {
			row, err := e.Deals.GetDeal(ctx, deal, storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			requireRecordAffordances(t, row.Writable, row.Archivable, expectedRecordActions{writable: writable, archivable: true})
		}},
		{"contact page", recordReadCost.contactPage, func(ctx context.Context, writable bool) {
			rows, _, err := e.Contacts.ListContacts(ctx, contacts.ListContactsInput{})
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 {
				t.Fatalf("contacts = %d, want 1", len(rows))
			}
			requireRecordAffordances(t, rows[0].Writable, rows[0].Archivable, expectedRecordActions{writable: writable, archivable: true})
		}},
		{"company page", recordReadCost.companyPage, func(ctx context.Context, writable bool) {
			rows, _, err := e.Contacts.ListCompanies(ctx, contacts.ListCompaniesInput{IDs: []ids.UUID{company.UUID}})
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 {
				t.Fatalf("companies = %d, want 1", len(rows))
			}
			requireRecordAffordances(t, rows[0].Writable, rows[0].Archivable, expectedRecordActions{writable: writable, archivable: true})
		}},
		{"deal page", recordReadCost.dealPage, func(ctx context.Context, writable bool) {
			rows, _, err := e.Deals.ListDeals(ctx, deals.ListDealsInput{})
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 {
				t.Fatalf("deals = %d, want 1", len(rows))
			}
			requireRecordAffordances(t, rows[0].Writable, rows[0].Archivable, expectedRecordActions{writable: writable, archivable: true})
		}},
		// Leads and projects carry no archive flag, so the edit flag's read is
		// all the update grant adds.
		{"lead", recordReadCost.lead, func(ctx context.Context, writable bool) {
			row, err := e.Contacts.GetLead(ctx, lead, storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			requireWritable(t, row.Writable, writable)
		}},
		{"project", recordReadCost.project, func(ctx context.Context, writable bool) {
			row, err := e.Projects.GetProject(ctx, project, storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			requireWritable(t, row.Writable, writable)
		}},
		{"lead page", recordReadCost.leadPage, func(ctx context.Context, writable bool) {
			rows, _, err := e.Contacts.ListLeads(ctx, contacts.ListLeadsInput{})
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 {
				t.Fatalf("leads = %d, want 1", len(rows))
			}
			requireWritable(t, rows[0].Writable, writable)
		}},
		{"project page", recordReadCost.projectPage, func(ctx context.Context, writable bool) {
			rows, _, err := e.Projects.ListProjects(ctx, projects.ListProjectsInput{})
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 {
				t.Fatalf("projects = %d, want 1", len(rows))
			}
			requireWritable(t, rows[0].Writable, writable)
		}},
	}
	for _, tc := range cases {
		// Both readers keep delete, so company domain checks and child reads match.
		var costs [2]int64
		for i, update := range []bool{false, true} {
			ctx, meter := database.WithStatementMeter(asTeam1Member(e, e.Rep1, recordAffordancePerms(recordWriteGrants{update: update, deleteGrant: true})))
			tc.read(ctx, update)
			cost := meter.Cost()
			if cost.Statements == 0 || cost.Failed != 0 {
				t.Fatalf("%s: invalid read cost %+v", tc.name, cost)
			}
			costs[i] = cost.Statements
		}
		t.Logf("%s: delete-only=%d, update+delete=%d statements", tc.name, costs[0], costs[1])
		if costs != tc.want {
			t.Errorf("%s: delete-only, update+delete = %v statements, want %v", tc.name, costs, tc.want)
		}
	}
}

// recordReadCost is each store read's statement count on a one-row fixture.
// Contacts, companies and deals answer both flags from one authority read, so
// adding the update grant costs nothing. A lead or project reader without the
// update grant skips its two authority statements.
var recordReadCost = struct {
	contact, company, deal, contactPage, companyPage, dealPage [2]int64
	lead, project, leadPage, projectPage                       [2]int64
}{
	contact: [2]int64{15, 15}, company: [2]int64{12, 12}, deal: [2]int64{9, 9},
	contactPage: [2]int64{12, 12}, companyPage: [2]int64{11, 11}, dealPage: [2]int64{7, 7},
	lead: [2]int64{6, 8}, project: [2]int64{5, 7}, leadPage: [2]int64{7, 9}, projectPage: [2]int64{4, 6},
}

type recordWriteGrants struct {
	update, deleteGrant bool
}

type expectedRecordActions struct {
	writable, archivable bool
}

func recordAffordancePerms(grants recordWriteGrants) principal.Permissions {
	perms := ownRecordArchivePerms()
	perms.RowScope = principal.RowScopeOwn
	for _, object := range []string{objContact, objCompany, objDeal, objLead, objProject} {
		grant := perms.Objects[object]
		grant.Read = true
		grant.Update, grant.Delete = grants.update, grants.deleteGrant
		perms.Objects[object] = grant
	}
	return perms
}

func requireWritable(t *testing.T, writable *bool, want bool) {
	t.Helper()
	if writable == nil {
		t.Fatal("missing writable flag")
	}
	if *writable != want {
		t.Errorf("writable = %t, want %t", *writable, want)
	}
}

func requireRecordAffordances(t *testing.T, writable, archivable *bool, want expectedRecordActions) {
	t.Helper()
	if writable == nil || archivable == nil {
		t.Fatal("missing writable or archivable flag")
	}
	if *writable != want.writable || *archivable != want.archivable {
		t.Errorf("writable/archivable = %t/%t, want %t/%t", *writable, *archivable, want.writable, want.archivable)
	}
}
