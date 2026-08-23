// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

// The companies a project is worked by, as a port the composition root fills.
//
// A project is work several companies do together — a customer, a partner, a
// subcontractor — so the companies are edges rather than one anchor column. The
// edges are `relationship` rows, and `people` owns that table, so this module
// asks rather than writes (ADR-0054 §3: a module never imports a sibling).
//
// Every seam here runs INSIDE the caller's transaction. That is what makes the
// company a fact of the create rather than a second write that can be missing:
// a project whose company edge failed to land is a project nobody's company
// page shows, and it must not be able to commit alone.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// AttachCompany puts one company on a project with a role, idempotently: an
// edge that already exists is left as it is rather than duplicated.
type AttachCompany func(
	ctx context.Context, tx pgx.Tx,
	projectID ids.ProjectID, organizationID ids.OrganizationID, role, by string,
) error

// ProjectCompanies lists the companies on a project, in the order they were
// attached, bounded by what the caller may see.
type ProjectCompanies func(
	ctx context.Context, tx pgx.Tx, projectID ids.ProjectID,
) ([]CompanyOnProject, error)

// CompanyOnProject is one company's place on a project.
type CompanyOnProject struct {
	OrganizationID ids.OrganizationID
	DisplayName    string
	Role           string
}

// CompaniesFrom adapts a reader that answers rows of some other shape into the
// port this module declares.
//
// It is here, and generic, because two callers wire this store — the
// composition root and the integration harness — and a second hand-written
// adapter is a second answer to "what is a company on a project". The rows'
// three fields are read through an interface rather than shared as a type: the
// port belongs to the module that ASKS, so the module that answers can add a
// field to its own row without changing what a project is told.
func CompaniesFrom[R CompanyRow](read func(context.Context, pgx.Tx, ids.ProjectID) ([]R, error)) ProjectCompanies {
	return func(ctx context.Context, tx pgx.Tx, projectID ids.ProjectID) ([]CompanyOnProject, error) {
		rows, err := read(ctx, tx, projectID)
		if err != nil {
			return nil, err
		}
		out := make([]CompanyOnProject, 0, len(rows))
		for _, r := range rows {
			out = append(out, CompanyOnProject{
				OrganizationID: r.Company(), DisplayName: r.Name(), Role: r.OnProjectAs(),
			})
		}
		return out, nil
	}
}

// CompanyRow is what CompaniesFrom needs of a row: which company, what it is
// called, and what it is to the project.
type CompanyRow interface {
	Company() ids.OrganizationID
	Name() string
	OnProjectAs() string
}

// CompanyRoleCustomer is the role a project's own client holds, and the role
// the first company on a project takes when nobody names one. Spelled here
// because the create path, the migration backfill and the wire vocabulary must
// agree about it.
const CompanyRoleCustomer = "customer"

// refusingAttachCompany is what an un-injected attach becomes: it refuses every
// company rather than admitting every one. A seam that failed OPEN here would
// create projects nobody's company page can find, which looks exactly like the
// project not existing.
func refusingAttachCompany() AttachCompany {
	return func(context.Context, pgx.Tx, ids.ProjectID, ids.OrganizationID, string, string) error {
		return errCompanySeamUnwired("the company attach")
	}
}

// refusingProjectCompanies is the read's twin. It refuses rather than answering
// an empty list: "this project has no companies" and "nobody wired the seam
// that would say" are different facts, and only one of them is worth showing a
// reader.
func refusingProjectCompanies() ProjectCompanies {
	return func(context.Context, pgx.Tx, ids.ProjectID) ([]CompanyOnProject, error) {
		return nil, errCompanySeamUnwired("the project-companies read")
	}
}

// errCompanySeamUnwired names the seam and what to construct instead, so an
// operator reading the log is told the fix rather than only the symptom.
func errCompanySeamUnwired(seam string) error {
	return errors.New("projects: " + seam + " seam was not injected; construct this " +
		"store through compose, which binds modules/people's relationship edges to it")
}
