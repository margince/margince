// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A project's companies are its live edges, and the pair an edge discloses is
// relationship.read's to grant. Reports, exports and the delivery continuity
// check read them that way, never through the stored project.company_id.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
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

// No report field that names a company reaches a seat without company.read.
// The row scope of a referenced column never stands in for its object grant.
// Read over the whole published vocabulary, so a new report or column cannot
// skip it.
func TestNoReportOffersACompanyToASeatWithoutCompanyRead(t *testing.T) {
	e := Setup(t)
	everything := []string{"deal", "lead", "activity", "contact", "project", "partner", "relationship", "company"}
	with := compose.AnalyticsSchemaFor(e.readerWith(everything...))
	without := compose.AnalyticsSchemaFor(e.readerWith(everything[:len(everything)-1]...))
	named := 0
	for key, entity := range with.Entities {
		for field := range entity.Fields {
			if !strings.HasSuffix(field, "company_id") {
				continue
			}
			named++
			if _, offered := without.Entities[key].Fields[field]; offered {
				t.Errorf("%s offers %s to a seat without company.read", key, field)
			}
		}
	}
	if named == 0 {
		t.Fatal("no report names a company, so this test swept nothing")
	}
	// The admin's schema lists every catalog report, whatever its record grant.
	reader := e.readerWith(everything[:len(everything)-1]...)
	ran := 0
	for key := range compose.AnalyticsSchemaFor(e.Admin()).Entities {
		if _, err := tryPrebuiltReport(e.Admin(), e, key, ""); err != nil {
			t.Fatalf("%s default plan as admin: %v", key, err)
		}
		columns, refused := defaultPlanColumns(reader, t, e, key)
		if !refused {
			ran++
		}
		for _, field := range columns {
			if strings.HasSuffix(field, "company_id") {
				t.Errorf("%s's default plan hands %s to a seat without company.read", key, field)
			}
		}
	}
	if ran == 0 {
		t.Fatal("every default plan was refused, so this test inspected nothing")
	}
	if _, refused := defaultPlanColumns(reader, t, e, "open-deals-per-company"); !refused {
		t.Error("open-deals-per-company ran its company grouping for a seat without company.read")
	}
	for _, plan := range []string{`{"group_by":["partner_company_id"]}`, `{"group_by":["company_id"]}`} {
		if _, err := tryPrebuiltReport(e.readerWith(everything[:len(everything)-1]...), e, "deals-by-stage", plan); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("deals-by-stage %s without company.read → %v, want ErrPermissionDenied", plan, err)
		}
	}
}

// A locked delivery read holds the account's edge set still until the
// decision commits. Neither taking a company off a project nor putting it on
// another can change the fact under it.
func TestALockedDeliveryReadHoldsTheAccountsEdges(t *testing.T) {
	e := Setup(t)
	f := seedMovedProject(t, e)
	admin := e.Admin()
	other, err := e.Projects.CreateProject(admin, projects.CreateProjectInput{
		Name: "Other delivery", CompanyID: f.current, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	locked, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- database.WithWorkspaceTx(admin, e.Pool, func(tx pgx.Tx) error {
			if _, err := contacts.LockCompanyTx(admin, tx, f.partner); err != nil {
				return err
			}
			if _, _, err := projects.CompanyDeliveryTx(admin, tx, f.partner, true); err != nil {
				return err
			}
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked
	waiting := func(what string, write func(context.Context) error) {
		t.Helper()
		ctx, cancel := context.WithTimeout(admin, time.Second)
		defer cancel()
		if err := write(ctx); err == nil {
			t.Errorf("%s committed under a locked delivery read", what)
		}
	}
	waiting("taking the partner off", func(ctx context.Context) error {
		return e.Contacts.RemoveProjectCompany(ctx, f.project, f.partner)
	})
	waiting("putting the partner on another project", func(ctx context.Context) error {
		_, err := e.Contacts.SetProjectCompany(ctx, contacts.SetProjectCompanyInput{
			ProjectID: ids.From[ids.ProjectKind](ids.UUID(other.Id)), CompanyID: f.partner, Role: "partner",
		})
		return err
	})
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := e.Contacts.RemoveProjectCompany(admin, f.project, f.partner); err != nil {
		t.Errorf("taking the partner off after the decision committed: %v", err)
	}
}

// defaultPlanColumns runs a catalog report with no plan of its own and
// returns the effective group-by, or refused when the seat may not run it.
func defaultPlanColumns(ctx context.Context, t *testing.T, e *Env, report string) (groupBy []string, refused bool) {
	t.Helper()
	out, err := compose.NewRegistry(e.Pool, compose.SendPath{}).Invoke(ctx, "run_report", []byte(`{"report":"`+report+`"}`))
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil, true
	}
	if err != nil {
		t.Fatalf("%s default plan: %v", report, err)
	}
	var envelope struct {
		Data struct {
			Plan struct {
				GroupBy []string `json:"group_by"`
			} `json:"plan"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &envelope); err != nil {
		t.Fatalf("decoding %s: %v", report, err)
	}
	return envelope.Data.Plan.GroupBy, false
}
