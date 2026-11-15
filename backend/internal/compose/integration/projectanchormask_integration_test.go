// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"reflect"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type projectAnchorExpectation struct {
	customer  string
	companies [][3]string
	masked    bool
	writable  bool
}

func checkProjectAnchor(t *testing.T, got crmcontracts.Project, want projectAnchorExpectation) {
	t.Helper()
	customer := ""
	if got.CompanyId != nil {
		customer = got.CompanyId.String()
	}
	if customer != want.customer {
		t.Errorf("company_id = %q, want %q", customer, want.customer)
	}
	if got.Companies == nil {
		t.Fatal("companies omitted, want an explicit list")
	}
	companies := make([][3]string, 0, len(*got.Companies))
	for _, one := range *got.Companies {
		companies = append(companies, [3]string{one.CompanyId.String(), one.DisplayName, one.Role})
	}
	if !reflect.DeepEqual(companies, want.companies) {
		t.Errorf("companies = %v, want %v", companies, want.companies)
	}
	if want.masked {
		if got.MaskedFields == nil || !reflect.DeepEqual(*got.MaskedFields, []string{"company_id"}) {
			t.Errorf("masked_fields = %v, want only company_id", got.MaskedFields)
		}
	} else if got.MaskedFields != nil {
		t.Errorf("masked_fields = %v, want omitted", got.MaskedFields)
	}
	if got.Writable == nil {
		t.Error("writable omitted")
	} else if *got.Writable != want.writable {
		t.Errorf("writable = %v, want %v", *got.Writable, want.writable)
	}
}

func TestProjectAnchorMaskIsIndependentOfVisibleCompanyEdges(t *testing.T) {
	for _, tc := range []struct {
		name                                                                                string
		private, partner, customer, removed, noCompany, noRelationships, readOnly, allScope bool
	}{
		{name: "visible customer"},
		{name: "hidden customer", private: true},
		{name: "hidden customer with visible partner", private: true, partner: true},
		{name: "hidden first customer with a later visible customer", private: true, customer: true},
		{name: "hidden anchor taken off leaves the visible customer first", private: true, customer: true, removed: true},
		{name: "no company grant", noCompany: true},
		{name: "no relationship grant", noRelationships: true},
		{name: "hidden anchor without relationship grant", private: true, noRelationships: true},
		{name: "read only project", private: true, readOnly: true},
		{name: "all scope does not bypass capture privacy", private: true, allScope: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := Setup(t)
			anchor := e.SeedCompany(t, "Original Customer", &e.Rep3)
			p := seedProject(e.Admin(), t, e, "Joint delivery", anchor, &e.Rep1)
			want := projectAnchorExpectation{companies: make([][3]string, 0), writable: !tc.readOnly}
			if !tc.private && !tc.noCompany && !tc.noRelationships {
				want.customer = anchor.String()
				want.companies = append(want.companies, [3]string{anchor.String(), "Original Customer", "customer"})
			}
			if tc.partner || tc.customer {
				company := e.SeedCompany(t, "Visible Collaborator", nil)
				role := "partner"
				if tc.customer {
					role = "customer"
				}
				if _, err := e.Contacts.SetProjectCompany(e.Admin(), contacts.SetProjectCompanyInput{
					ProjectID: p.ID, CompanyID: companyIDOf(company), Role: role,
				}); err != nil {
					t.Fatal(err)
				}
				want.companies = append(want.companies, [3]string{company.String(), "Visible Collaborator", role})
				if tc.removed {
					want.customer = company.String()
				}
			}
			if tc.removed {
				if err := e.Contacts.RemoveProjectCompany(e.Admin(), p.ID, companyIDOf(anchor)); err != nil {
					t.Fatal(err)
				}
			}
			if tc.private {
				e.MakeCapturePrivate(t, "company", anchor, e.Rep3)
			}
			want.masked = (tc.private && !tc.removed) || tc.noCompany || tc.noRelationships
			objects := map[string]principal.ObjectGrant{
				"project":      {Read: true, Update: !tc.readOnly},
				"company":      {Read: !tc.noCompany},
				"relationship": {Read: !tc.noRelationships},
			}
			scope := principal.RowScopeOwn
			if tc.allScope {
				scope = principal.RowScopeAll
			}
			reader := OnSeat(e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
				RoleKeys: []string{"rep"}, Objects: objects, RowScope: scope,
			}), principal.SeatFull)
			got, err := e.Projects.GetProject(reader, p.ID, storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			checkProjectAnchor(t, got, want)
		})
	}
}

// listedCustomer is what one list row says about its customer.
type listedCustomer struct {
	customer string
	masked   bool
}

// A list row carries no company list. Its company_id and mask must still
// match what a single read answers.
func TestProjectListRowsNameOnlyAReadableFirstCustomer(t *testing.T) {
	e := Setup(t)
	hidden := e.SeedCompany(t, "Private Customer", &e.Rep3)
	open := e.SeedCompany(t, "Open Customer", nil)
	later := e.SeedCompany(t, "Later Customer", nil)
	hiddenFirst := seedProject(e.Admin(), t, e, "Hidden first customer", hidden, &e.Rep1)
	openFirst := seedProject(e.Admin(), t, e, "Open first customer", open, &e.Rep1)
	hiddenTakenOff := seedProject(e.Admin(), t, e, "Hidden customer taken off", hidden, &e.Rep1)
	noCustomer := seedProject(e.Admin(), t, e, "Partner only", open, &e.Rep1)
	for _, project := range []ids.ProjectID{hiddenFirst.ID, hiddenTakenOff.ID} {
		if _, err := e.Contacts.SetProjectCompany(e.Admin(), contacts.SetProjectCompanyInput{
			ProjectID: project, CompanyID: companyIDOf(later), Role: "customer",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.Contacts.RemoveProjectCompany(e.Admin(), hiddenTakenOff.ID, companyIDOf(hidden)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Contacts.SetProjectCompany(e.Admin(), contacts.SetProjectCompanyInput{
		ProjectID: noCustomer.ID, CompanyID: companyIDOf(open), Role: "partner",
	}); err != nil {
		t.Fatal(err)
	}
	e.MakeCapturePrivate(t, "company", hidden, e.Rep3)
	withheld := listedCustomer{masked: true}
	for _, tc := range []struct {
		name                     string
		relationships, companies bool
		want                     map[ids.ProjectID]listedCustomer
	}{
		{name: "with both grants", relationships: true, companies: true, want: map[ids.ProjectID]listedCustomer{
			hiddenFirst.ID: withheld, openFirst.ID: {customer: open.String()},
			hiddenTakenOff.ID: {customer: later.String()}, noCustomer.ID: {},
		}},
		{name: "without the company grant", relationships: true, want: map[ids.ProjectID]listedCustomer{
			hiddenFirst.ID: withheld, openFirst.ID: withheld, hiddenTakenOff.ID: withheld, noCustomer.ID: {},
		}},
		{name: "without the relationship grant", companies: true, want: map[ids.ProjectID]listedCustomer{
			hiddenFirst.ID: withheld, openFirst.ID: withheld, hiddenTakenOff.ID: withheld, noCustomer.ID: withheld,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := OnSeat(e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
				RoleKeys: []string{"rep"}, RowScope: principal.RowScopeOwn,
				Objects: map[string]principal.ObjectGrant{
					"project": {Read: true}, "company": {Read: tc.companies}, "relationship": {Read: tc.relationships},
				},
			}), principal.SeatFull)
			rows, _, err := e.Projects.ListProjects(reader, projects.ListProjectsInput{})
			if err != nil {
				t.Fatal(err)
			}
			got := map[ids.ProjectID]listedCustomer{}
			for _, row := range rows {
				one := listedCustomer{masked: row.MaskedFields != nil}
				if row.MaskedFields != nil && !reflect.DeepEqual(*row.MaskedFields, []string{"company_id"}) {
					t.Errorf("project %s: masked_fields = %v, want only company_id", row.Id, *row.MaskedFields)
				}
				if row.CompanyId != nil {
					one.customer = row.CompanyId.String()
				}
				got[ids.From[ids.ProjectKind](ids.UUID(row.Id))] = one
			}
			for project, want := range tc.want {
				if got[project] != want {
					t.Errorf("project %s: %+v, want %+v", project, got[project], want)
				}
			}
		})
	}
}

// Ordering by company reads the same first customer company_id names, so a
// withheld one sorts last instead of by a later customer's name.
func TestProjectsSortByTheirReadableFirstCustomer(t *testing.T) {
	e := Setup(t)
	hidden := e.SeedCompany(t, "Aaron Private", &e.Rep3)
	zeta := e.SeedCompany(t, "Zeta Open", nil)
	alma := e.SeedCompany(t, "Alma Open", nil)
	hiddenFirst := seedProject(e.Admin(), t, e, "Hidden first", hidden, &e.Rep1)
	if _, err := e.Contacts.SetProjectCompany(e.Admin(), contacts.SetProjectCompanyInput{
		ProjectID: hiddenFirst.ID, CompanyID: companyIDOf(alma), Role: "customer",
	}); err != nil {
		t.Fatal(err)
	}
	zetaFirst := seedProject(e.Admin(), t, e, "Zeta first", zeta, &e.Rep1)
	almaFirst := seedProject(e.Admin(), t, e, "Alma first", alma, &e.Rep1)
	e.MakeCapturePrivate(t, "company", hidden, e.Rep3)
	reader := OnSeat(e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{"rep"}, RowScope: principal.RowScopeOwn,
		Objects: map[string]principal.ObjectGrant{
			"project": {Read: true}, "company": {Read: true}, "relationship": {Read: true},
		},
	}), principal.SeatFull)
	assertIDOrder(t, projectIDsIn(reader, t, e, "company_id"),
		[]ids.UUID{almaFirst.ID.UUID, zetaFirst.ID.UUID, hiddenFirst.ID.UUID},
		"Alma, then Zeta, then the withheld first customer last")
}

func TestProjectMutationEchoesKeepTheHiddenAnchorMask(t *testing.T) {
	e := Setup(t)
	anchor := e.SeedCompany(t, "Private Customer", &e.Rep3)
	p := seedProject(e.Admin(), t, e, "Delivery", anchor, &e.Rep1)
	e.MakeCapturePrivate(t, "company", anchor, e.Rep3)
	reader := OnSeat(e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{"rep"}, RowScope: principal.RowScopeOwn,
		Objects: map[string]principal.ObjectGrant{
			"project": {Read: true, Update: true, Delete: true},
			"company": {Read: true}, "relationship": {Read: true},
		},
	}), principal.SeatFull)
	want := projectAnchorExpectation{companies: make([][3]string, 0), masked: true, writable: true}
	noChange, err := e.Projects.UpdateProject(reader, p.ID, projects.UpdateProjectInput{})
	if err != nil {
		t.Fatal(err)
	}
	checkProjectAnchor(t, noChange, want)
	name := "Revised delivery"
	updated, err := e.Projects.UpdateProject(reader, p.ID, projects.UpdateProjectInput{Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	checkProjectAnchor(t, updated, want)
	advanced, err := e.Projects.AdvanceProjectPhase(reader, p.ID, projects.AdvanceProjectPhaseInput{ToPhase: "pursuing"})
	if err != nil {
		t.Fatal(err)
	}
	checkProjectAnchor(t, advanced, want)
	archived, err := e.Projects.ArchiveProject(reader, p.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	want.writable = false
	checkProjectAnchor(t, archived, want)
}

func TestProjectCreateEchoWithoutEdgeGrantWithholdsTheCustomer(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Visible Customer", nil)
	creator := OnSeat(e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{"rep"}, RowScope: principal.RowScopeOwn,
		Objects: map[string]principal.ObjectGrant{
			"project": {Create: true, Read: true, Update: true},
			"company": {Read: true},
		},
	}), principal.SeatFull)
	got, err := e.Projects.CreateProject(creator, projects.CreateProjectInput{
		Name: "Delivery without edge access", CompanyID: companyIDOf(company), Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	checkProjectAnchor(t, got, projectAnchorExpectation{
		companies: make([][3]string, 0), masked: true, writable: true,
	})
}
