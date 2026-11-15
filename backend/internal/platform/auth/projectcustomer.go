// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// A project's companies are relationship edges. A project's company_id is its
// first customer edge (ProjectCustomerSQL), and a filter or hop by company
// matches any live edge (ProjectCompanyEdgesSQL). Both are bounded by
// ProjectCompanyPredicates, so a pair one surface withholds is withheld on all.

import (
	"context"
	"errors"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// ProjectCompanyKind is the relationship kind of a project's company edges,
// and ProjectCustomerRole the role a project's client holds on one.
const (
	ProjectCompanyKind  = "project_company"
	ProjectCustomerRole = "customer"
)

// The predicates a bound answers with when it admits every row or none.
const (
	sqlAdmitAll  = "true"
	sqlAdmitNone = "false"
)

// ProjectCompanyBounds is what one caller may learn from a project's company
// edges. A predicate is set whenever its flag holds, and never empty.
type ProjectCompanyBounds struct {
	// EdgesRead is false without relationship.read: no edge may be read.
	EdgesRead bool
	// CompaniesRead is false without company.read. The edges may still be
	// read, so a caller can learn a customer exists but not which.
	CompaniesRead bool
	Edge, Company string
}

// ProjectCompanyPredicates bounds a read of project company edges on
// edgeAlias joined to company on companyAlias.
//
// An edge discloses that a project and a company work together, which
// relationship.read governs. The company object grant is checked before its
// row scope, because row_scope=all admits every company row.
func ProjectCompanyPredicates(
	ctx context.Context, edgeAlias, companyAlias string, arg func(any) int,
) (ProjectCompanyBounds, error) {
	edge, err := EdgeReadScope(ctx, edgeAlias, arg)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return ProjectCompanyBounds{}, nil
	}
	if err != nil {
		return ProjectCompanyBounds{}, err
	}
	if edge == "" {
		edge = sqlAdmitAll
	}
	if !ReadGranted(ctx, tableCompany) {
		return ProjectCompanyBounds{EdgesRead: true, Edge: edge, Company: sqlAdmitNone}, nil
	}
	company, err := ScopeClauseFor(ctx, tableCompany, companyAlias, arg)
	if err != nil {
		return ProjectCompanyBounds{}, err
	}
	if company == "" {
		company = sqlAdmitAll
	}
	return ProjectCompanyBounds{EdgesRead: true, CompaniesRead: true, Edge: edge, Company: company}, nil
}

// ProjectCustomer is the SQL for one project's company_id, as one caller may
// read it.
type ProjectCustomer struct {
	// EdgesRead is false without relationship.read. Readable is then a NULL
	// and Present must not be used: nothing about the customer is known.
	EdgesRead bool
	// Readable is a uuid expression: the first live customer edge's company
	// when this caller may read it, NULL otherwise.
	Readable string
	// Present is a boolean expression: the project has a live customer edge.
	Present string
}

// ProjectCustomerSQL renders the first-customer expressions for the project
// whose id is projectRef, a column reference of the caller's statement.
//
// A withheld customer is NULL in SQL, so its id never leaves the database. A
// later visible customer never stands in for it.
func ProjectCustomerSQL(ctx context.Context, projectRef string, arg func(any) int) (ProjectCustomer, error) {
	bounds, err := ProjectCompanyPredicates(ctx, "customer_edge", "customer_company", arg)
	if err != nil || !bounds.EdgesRead {
		return ProjectCustomer{Readable: "NULL::uuid"}, err
	}
	live := `FROM relationship customer_edge
		  JOIN company customer_company ON customer_company.id = customer_edge.company_id
		 WHERE customer_edge.kind = '` + ProjectCompanyKind + `' AND customer_edge.project_id = ` + projectRef + `
		   AND customer_edge.role = '` + ProjectCustomerRole + `'
		   AND customer_edge.archived_at IS NULL AND customer_company.archived_at IS NULL`
	return ProjectCustomer{
		EdgesRead: true,
		Readable: `(SELECT CASE WHEN (` + bounds.Edge + `) AND (` + bounds.Company + `)
		         THEN customer_edge.company_id END ` + live + `
		 ORDER BY customer_edge.created_at, customer_edge.id LIMIT 1)`,
		Present: `EXISTS (SELECT 1 ` + live + `)`,
	}, nil
}

// The columns of ProjectCompanyEdges.Source a caller correlates against.
const (
	ProjectEdgeProject = "project_edge.project_id"
	ProjectEdgeCompany = "project_edge.company_id"
)

// ProjectCompanyEdges is the readable live project-company edges, as a
// from-and-where clause a caller extends with its own correlation.
type ProjectCompanyEdges struct {
	// Readable is false without relationship.read or company.read. No edge
	// is then readable, and Source must not be used.
	Readable bool
	// Source runs from the from clause into the where clause. A caller appends
	// `AND <ProjectEdgeProject or ProjectEdgeCompany> = <its own reference>`.
	Source string
}

// ProjectCompanyEdgesSQL renders every live project-company edge this caller
// may read, whatever its role. A filter or hop by company matches through it.
func ProjectCompanyEdgesSQL(ctx context.Context, arg func(any) int) (ProjectCompanyEdges, error) {
	// Asked before any predicate binds an argument, so a caller rendering no
	// clause leaves no parameter behind in its statement.
	if !ReadGranted(ctx, tableCompany) {
		return ProjectCompanyEdges{}, nil
	}
	bounds, err := ProjectCompanyPredicates(ctx, "project_edge", "project_company", arg)
	if err != nil || !bounds.CompaniesRead {
		return ProjectCompanyEdges{}, err
	}
	return ProjectCompanyEdges{Readable: true, Source: `FROM relationship project_edge
		  JOIN company project_company ON project_company.id = project_edge.company_id
		 WHERE project_edge.kind = '` + ProjectCompanyKind + `'
		   AND project_edge.archived_at IS NULL AND project_company.archived_at IS NULL
		   AND (` + bounds.Edge + `) AND (` + bounds.Company + `)`}, nil
}
