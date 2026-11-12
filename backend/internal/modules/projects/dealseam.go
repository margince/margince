// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

// The deal a new project comes from, as a port the composition root fills.
// `deals` owns the deal row and its project pointer, so this module asks
// rather than writes.

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CheckDeal refuses a deal a project on company could not be created from,
// and writes nothing. The approval preview runs it; AttachDeal runs it too.
type CheckDeal func(ctx context.Context, tx pgx.Tx, dealID ids.DealID, company ids.CompanyID) error

// AttachDeal points a deal at a project inside the caller's transaction, under
// the same write check and audit a deal-side link takes.
type AttachDeal func(ctx context.Context, tx pgx.Tx, dealID ids.DealID, projectID ids.ProjectID, company ids.CompanyID) error

// errDealSeamUnwired is what an un-injected seam answers: a create naming a
// deal fails rather than committing a project the deal does not point at.
var errDealSeamUnwired = errors.New("projects: the deal link seam was not injected; build this store with WithDealLink")

func refusingCheckDeal() CheckDeal {
	return func(context.Context, pgx.Tx, ids.DealID, ids.CompanyID) error { return errDealSeamUnwired }
}

func refusingAttachDeal() AttachDeal {
	return func(context.Context, pgx.Tx, ids.DealID, ids.ProjectID, ids.CompanyID) error {
		return errDealSeamUnwired
	}
}

// DealCompanyMismatchError maps to 422: the deal belongs to a company the new
// project is not on.
type DealCompanyMismatchError struct{}

func (e *DealCompanyMismatchError) Error() string {
	return "the deal belongs to another company; pick the deal's company for the project"
}

// FieldFault places the refusal on the create body's deal field.
func (e *DealCompanyMismatchError) FieldFault() (field, code, message string) {
	return "deal_id", "project_company_mismatch", e.Error()
}

// DealNotLinkableError maps to 422: the deal cannot become the new project's
// deal. A link it already has is changed on the deal, not by a create.
type DealNotLinkableError struct {
	Code    string
	Message string
}

func (e *DealNotLinkableError) Error() string { return e.Message }

// FieldFault places the refusal on the create body's deal field.
func (e *DealNotLinkableError) FieldFault() (field, code, message string) {
	return "deal_id", e.Code, e.Message
}

// RefuseDealForNewProject admits only an open deal that names no project. A
// withheld project pointer counts as named, so a create never moves a deal
// off a project its caller cannot see.
func RefuseDealForNewProject(status string, namesProject bool, masked *[]string) error {
	if namesProject || (masked != nil && slices.Contains(*masked, "project_id")) {
		return &DealNotLinkableError{Code: "deal_has_project",
			Message: "the deal already belongs to a project; change its project on the deal instead"}
	}
	if status != "open" {
		return &DealNotLinkableError{Code: "deal_not_open",
			Message: "only an open deal can start a project here; create the project, then attach the deal on the deal page"}
	}
	return nil
}

// createSeams are the cross-module edges a create runs inside its transaction.
type createSeams struct {
	attachCompany    AttachCompany
	projectCompanies ProjectCompanies
	attachDeal       AttachDeal
}

func (s *Store) createSeams() createSeams {
	return createSeams{attachCompany: s.attachCompany, projectCompanies: s.projectCompanies, attachDeal: s.attachDeal}
}

// WithDealLink wires the seams that check and point a deal at the project
// being created. Compose binds them to modules/deals, which owns the deal row.
func (s *Store) WithDealLink(check CheckDeal, attach AttachDeal) *Store {
	if check != nil {
		s.checkDeal = check
	}
	if attach != nil {
		s.attachDeal = attach
	}
	return s
}
