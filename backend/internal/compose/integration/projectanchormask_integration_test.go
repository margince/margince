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
		name                                                                       string
		private, partner, customer, noCompany, noRelationships, readOnly, allScope bool
	}{
		{name: "visible customer"},
		{name: "hidden customer", private: true},
		{name: "hidden customer with visible partner", private: true, partner: true},
		// company_id here is the second, visible customer while masked_fields
		// still names company_id: current behaviour, tracked in issue #11671.
		{name: "hidden anchor with visible customer", private: true, customer: true},
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
					want.customer = company.String()
				}
				if _, err := e.Contacts.SetProjectCompany(e.Admin(), contacts.SetProjectCompanyInput{
					ProjectID: p.ID, CompanyID: companyIDOf(company), Role: role,
				}); err != nil {
					t.Fatal(err)
				}
				want.companies = append(want.companies, [3]string{company.String(), "Visible Collaborator", role})
			}
			if tc.private {
				e.MakeCapturePrivate(t, "company", anchor, e.Rep3)
			}
			want.masked = tc.private || tc.noCompany
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

// A list page reads no company edges, so maskProjects keeps a hidden anchor's
// id off the row.
func TestProjectListRowsMaskOnlyTheHiddenAnchor(t *testing.T) {
	e := Setup(t)
	hidden := e.SeedCompany(t, "Private Customer", &e.Rep3)
	visible := e.SeedCompany(t, "Open Customer", nil)
	hiddenProject := seedProject(e.Admin(), t, e, "Hidden anchor delivery", hidden, &e.Rep1)
	visibleProject := seedProject(e.Admin(), t, e, "Open anchor delivery", visible, &e.Rep1)
	e.MakeCapturePrivate(t, "company", hidden, e.Rep3)
	reader := OnSeat(e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{"rep"}, RowScope: principal.RowScopeOwn,
		Objects: map[string]principal.ObjectGrant{
			"project": {Read: true}, "company": {Read: true}, "relationship": {Read: true},
		},
	}), principal.SeatFull)
	rows, _, err := e.Projects.ListProjects(reader, projects.ListProjectsInput{})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[ids.ProjectID]crmcontracts.Project{}
	for _, row := range rows {
		seen[ids.From[ids.ProjectKind](ids.UUID(row.Id))] = row
	}
	got, ok := seen[hiddenProject.ID]
	if !ok {
		t.Fatal("the hidden-anchor project is missing from the list")
	}
	if got.CompanyId != nil || got.MaskedFields == nil || !reflect.DeepEqual(*got.MaskedFields, []string{"company_id"}) {
		t.Errorf("hidden anchor row: company_id = %v, masked_fields = %v, want null and only company_id", got.CompanyId, got.MaskedFields)
	}
	got, ok = seen[visibleProject.ID]
	if !ok {
		t.Fatal("the visible-anchor project is missing from the list")
	}
	if got.CompanyId == nil || got.CompanyId.String() != visible.String() || got.MaskedFields != nil {
		t.Errorf("visible anchor row: company_id = %v, masked_fields = %v, want %s and no mask", got.CompanyId, got.MaskedFields, visible)
	}
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

func TestProjectCreateEchoWithoutEdgeGrantKeepsVisibleAnchorUnmasked(t *testing.T) {
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
		companies: make([][3]string, 0), writable: true,
	})
}
