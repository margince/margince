// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"testing"

	"github.com/gradionhq/margince/backend/internal/modules/people"
	"github.com/gradionhq/margince/backend/internal/modules/projects"
	"github.com/gradionhq/margince/backend/internal/platform/database/storekit"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// Three companies building one project is the case this edge exists for: a
// delivery has a customer, a partner and a subcontractor on it, and a model
// that admits only one of them forces the other two out of the record they are
// working in.
func TestThreeCompaniesWorkOneProjectAndEachPageFindsIt(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	compA := e.SeedOrg(t, "Alpha Werke", nil)
	compB := e.SeedOrg(t, "Beta Systeme", nil)
	compC := e.SeedOrg(t, "Gamma Bau", nil)

	p := seedProject(admin, t, e, "Joint rollout", compA, nil)
	for _, on := range []struct {
		org  ids.UUID
		role string
	}{{compB, "partner"}, {compC, "subcontractor"}} {
		if _, err := e.People.SetProjectCompany(admin, people.SetProjectCompanyInput{
			ProjectID: p.ID, OrganizationID: orgIDOf(on.org), Role: on.role,
		}); err != nil {
			t.Fatalf("put %s on the project: %v", on.role, err)
		}
	}

	// The project answers with all three, and organization_id still names the
	// customer — the one company most readers mean.
	got, err := e.Projects.GetProject(admin, p.ID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got.Organizations == nil || len(*got.Organizations) != 3 {
		t.Fatalf("the project lists %v companies, want all three", got.Organizations)
	}
	roles := map[string]string{}
	for _, one := range *got.Organizations {
		roles[one.DisplayName] = one.Role
	}
	for name, want := range map[string]string{
		"Alpha Werke": projects.CompanyRoleCustomer, "Beta Systeme": "partner", "Gamma Bau": "subcontractor",
	} {
		if roles[name] != want {
			t.Errorf("%s is on the project as %q, want %q", name, roles[name], want)
		}
	}
	if got.OrganizationId == nil || ids.UUID(*got.OrganizationId) != compA {
		t.Errorf("organization_id = %v, want the customer Alpha Werke", got.OrganizationId)
	}

	// And every one of the three finds the project on its OWN company page —
	// which is the whole point: a partner who cannot see the delivery they are
	// on has been told it is not theirs.
	svc := orgSurfaceService(e)
	for _, org := range []ids.UUID{compA, compB, compC} {
		page, err := svc.Assemble(admin, orgIDOf(org))
		if err != nil {
			t.Fatalf("assemble the page for %v: %v", org, err)
		}
		if page.Projects == nil || len(*page.Projects) != 1 {
			t.Errorf("company %v lists %v projects, want the joint one", org, page.Projects)
			continue
		}
		if (*page.Projects)[0].Name != "Joint rollout" {
			t.Errorf("company %v lists %q, want the joint rollout", org, (*page.Projects)[0].Name)
		}
	}
}

// A project keeps at least one company. Taking the last one off would leave
// work nobody is doing, reachable from no company page — so it is refused, and
// the refusal names the field.
func TestTheLastCompanyCannotBeTakenOffAProject(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	compA := e.SeedOrg(t, "Alpha Werke", nil)
	compB := e.SeedOrg(t, "Beta Systeme", nil)
	p := seedProject(admin, t, e, "Joint rollout", compA, nil)

	if _, err := e.People.SetProjectCompany(admin, people.SetProjectCompanyInput{
		ProjectID: p.ID, OrganizationID: orgIDOf(compB), Role: "partner",
	}); err != nil {
		t.Fatal(err)
	}
	// Two on, so one may come off.
	if err := e.People.RemoveProjectCompany(admin, p.ID, orgIDOf(compB)); err != nil {
		t.Fatalf("taking the partner off a project with two companies: %v", err)
	}
	// One left, so it may not.
	err := e.People.RemoveProjectCompany(admin, p.ID, orgIDOf(compA))
	var last *people.LastProjectCompanyError
	if !errors.As(err, &last) {
		t.Fatalf("taking the last company off answered %v, want the refusal", err)
	}

	// A company that was never on the project is not-found, not the last-company
	// refusal: the two say different things to a caller.
	compC := e.SeedOrg(t, "Gamma Bau", nil)
	if err := e.People.RemoveProjectCompany(admin, p.ID, orgIDOf(compC)); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("taking off a company that was never on it answered %v, want not-found", err)
	}
}
