// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A project's companies are its live edges, and the pair an edge discloses is
// relationship.read's to grant. Reports, exports and the delivery continuity
// check read them that way, never through the stored project.company_id.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/projectexport"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// movedProject is a delivering project whose first customer was taken off.
// Its stored company_id still names old. Current is its customer, and partner
// works it beside them.
type movedProject struct {
	project               ids.ProjectID
	old, current, partner ids.CompanyID
}

func seedMovedProject(t *testing.T, e *Env) movedProject {
	t.Helper()
	admin := e.Admin()
	f := movedProject{
		old:     companyIDOf(e.SeedCompany(t, "Old Customer", nil)),
		current: companyIDOf(e.SeedCompany(t, "Current Customer", nil)),
		partner: companyIDOf(e.SeedCompany(t, "Partner Co", nil)),
	}
	created, err := e.Projects.CreateProject(admin, projects.CreateProjectInput{
		Name: "Moved delivery", CompanyID: f.old, Phase: projects.PhaseDelivering, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.project = ids.From[ids.ProjectKind](ids.UUID(created.Id))
	for company, role := range map[ids.CompanyID]string{f.current: "customer", f.partner: "partner"} {
		if _, err := e.Contacts.SetProjectCompany(admin, contacts.SetProjectCompanyInput{
			ProjectID: f.project, CompanyID: company, Role: role,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.Contacts.RemoveProjectCompany(admin, f.project, f.old); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestAProjectReportFiltersByAnyLiveCompanyOnTheProject(t *testing.T) {
	e := Setup(t)
	f := seedMovedProject(t, e)
	reader := e.readerWith("project", "company", "relationship")
	count := func(company ids.CompanyID) string {
		rows := runPrebuiltReport(reader, t, e, "projects-by-phase", `{"filters":{"company_id":"`+company.String()+`"}}`)
		if len(rows) == 0 {
			return "0"
		}
		return cell(rows[0], "projects")
	}
	for company, want := range map[ids.CompanyID]string{f.current: "1", f.partner: "1", f.old: "0"} {
		if got := count(company); got != want {
			t.Errorf("projects with company %s = %s, want %s", company, got, want)
		}
	}
	grouped := runPrebuiltReport(reader, t, e, "projects-by-phase", `{"group_by":["company_id"]}`)
	if len(grouped) != 1 || cell(grouped[0], "company_id") != f.current.String() {
		t.Errorf("grouping by company = %v, want the one row under the current customer", grouped)
	}
}

// The partner filter keeps the project even when its first customer is
// capture-private to someone else. The customer column's mask governs what the
// report displays, never which rows the filter keeps.
func TestAProjectReportFilterIsNotNarrowedByAnUnreadableFirstCustomer(t *testing.T) {
	e := Setup(t)
	f := seedMovedProject(t, e)
	e.MakeCapturePrivate(t, "company", f.current.UUID, e.Rep3)
	reader := e.readerWith("project", "company", "relationship")
	rows := runPrebuiltReport(reader, t, e, "projects-by-phase", `{"filters":{"company_id":"`+f.partner.String()+`"}}`)
	if len(rows) != 1 || cell(rows[0], "projects") != "1" {
		t.Errorf("projects with the partner, first customer private = %v, want the one project", rows)
	}
}

// A seat that may read edges but not companies is refused the company, so it
// is never handed a company id.
func TestAProjectReportRefusesTheCompanyToASeatWithoutCompanyRead(t *testing.T) {
	e := Setup(t)
	f := seedMovedProject(t, e)
	reader := e.readerWith("project", "relationship")
	for _, plan := range []string{
		`{"group_by":["company_id"]}`,
		`{"filters":{"company_id":"` + f.current.String() + `"}}`,
	} {
		if _, err := tryPrebuiltReport(reader, e, "projects-by-phase", plan); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("%s without company.read → %v, want ErrPermissionDenied", plan, err)
		}
	}
}

func TestAProjectReportRefusesTheCompanyToASeatWithoutRelationshipRead(t *testing.T) {
	e := Setup(t)
	f := seedMovedProject(t, e)
	reader := e.readerWith("project", "company")
	for _, plan := range []string{
		`{"group_by":["company_id"]}`,
		`{"filters":{"company_id":"` + f.current.String() + `"}}`,
	} {
		if _, err := tryPrebuiltReport(reader, e, "projects-by-phase", plan); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("%s without relationship.read → %v, want ErrPermissionDenied", plan, err)
		}
	}
}

func TestAnExportedProjectNamesItsReadableFirstCustomer(t *testing.T) {
	e := Setup(t)
	f := seedMovedProject(t, e)
	exported := func(ctx context.Context) any {
		t.Helper()
		rows := [][]any{{[16]byte(f.project.UUID), [16]byte(f.old.UUID)}}
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			return projectexport.WriteCustomers(ctx, tx, "project", []string{"id", "company_id"}, rows)
		}); err != nil {
			t.Fatal(err)
		}
		return rows[0][1]
	}
	if got := exported(e.readerWith("project", "company", "relationship")); got != [16]byte(f.current.UUID) {
		t.Errorf("exported company_id = %v, want the current customer %s", got, f.current)
	}
	if got := exported(e.readerWith("project", "company")); got != nil {
		t.Errorf("exported company_id for a seat without relationship.read = %v, want nothing", got)
	}
}

func TestDeliveryContinuityCountsOnlyTheCompaniesOnAProject(t *testing.T) {
	e := Setup(t)
	f := seedMovedProject(t, e)
	admin := e.Admin()
	active := func(ctx context.Context, company ids.CompanyID) bool {
		t.Helper()
		var delivering bool
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			var err error
			delivering, _, err = projects.CompanyDeliveryTx(ctx, tx, company, false)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return delivering
	}
	for company, want := range map[ids.CompanyID]bool{f.current: true, f.partner: true, f.old: false} {
		if got := active(admin, company); got != want {
			t.Errorf("company %s in active delivery = %v, want %v", company, got, want)
		}
	}
	// Whether an account still has delivery is the account's fact: a seat that
	// may not read edges still finds the account's ending blocked.
	if !active(e.readerWith("project", "company"), f.current) {
		t.Error("a seat without relationship.read found no active delivery on a delivering account")
	}
}
