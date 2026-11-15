// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

// The project read mask, the sibling of fieldmask.go's deal mask and for the
// same reason: a projection that carries a reference to ANOTHER record must
// withhold the one its reader could not open, or the record becomes an
// existence oracle over rows that reader's own reads would refuse.
//
// A project has one such reference: company_id, its first customer edge. That company is capture-private until somebody promotes it
// (platform/auth rowscope.go), while the project hanging off it is read by
// every seat holding the object grant. So the two disagree by design, and this
// is where the wire answer is brought back in line with what the reader may
// actually see.

import (
	"context"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// filterCompanyIDOnProject is masked_fields' name for the anchor
// reference. It is the wire member's own name, which is what a client reads
// the mask against.
const filterCompanyIDOnProject = "company_id"

// maskProjectForCaller applies the read mask to ONE row about to leave the
// store.
// It is a METHOD rather than a free function so every single-project answer
// carries the company list without each of the six call sites remembering to
// ask: a project read whose companies were missing looks exactly like a project
// with no companies, and there is no field a reader could check to tell.
func (s *Store) maskProjectForCaller(ctx context.Context, tx pgx.Tx, p crmcontracts.Project) (crmcontracts.Project, error) {
	one := []crmcontracts.Project{p}
	if err := maskProjects(ctx, tx, one, s.projectCustomers); err != nil {
		return crmcontracts.Project{}, err
	}
	return s.withCompanies(ctx, tx, one[0])
}

// withCompanies fills a single project's company list from the edges.
//
// Only the single-project reads carry the list. A list page would need one more
// statement per page for a field no list column renders, and a page that
// answered it per row would be a probe per row — the thing maskProjects exists
// to avoid.
func (s *Store) withCompanies(ctx context.Context, tx pgx.Tx, p crmcontracts.Project) (crmcontracts.Project, error) {
	return fillCompanies(ctx, tx, p, s.projectCompanies)
}

// fillCompanies is withCompanies for a caller that holds the seam itself — the
// create path, which runs inside a transaction the store did not open.
// company_id and its mask are maskProjects' answer and stay as they are.
func fillCompanies(
	ctx context.Context, tx pgx.Tx, p crmcontracts.Project, companies ProjectCompanies,
) (crmcontracts.Project, error) {
	on, err := companies(ctx, tx, ids.From[ids.ProjectKind](ids.UUID(p.Id)))
	if err != nil {
		return crmcontracts.Project{}, err
	}
	listed := make([]crmcontracts.ProjectCompany, 0, len(on))
	for _, one := range on {
		listed = append(listed, crmcontracts.ProjectCompany{
			CompanyId:   openapi_types.UUID(one.CompanyID.UUID),
			DisplayName: one.DisplayName,
			Role:        one.Role,
		})
	}
	p.Companies = &listed
	return p, nil
}

// maskProjects stamps each row's write flag and sets company_id to the
// project's first customer edge, null and masked when this reader may not
// read it. One statement answers the whole page, never a probe per row.
func maskProjects(
	ctx context.Context, tx pgx.Tx, projects []crmcontracts.Project, customersOf ProjectCustomers,
) error {
	// Whether each project is this caller's to change, one statement for the
	// page. A project keeps the owner scope on writes even though every seat
	// reads it, so the flag is the only thing a client can draw its edit
	// affordances from without guessing.
	if _, err := auth.StampWritable(ctx, tx, "project", projects,
		func(p crmcontracts.Project) ids.UUID { return ids.UUID(p.Id) },
		func(p *crmcontracts.Project, may bool) { p.Writable = &may }); err != nil {
		return err
	}
	projectIDs := make([]ids.ProjectID, 0, len(projects))
	for _, p := range projects {
		projectIDs = append(projectIDs, ids.From[ids.ProjectKind](ids.UUID(p.Id)))
	}
	customers, edgesRead, err := customersOf(ctx, tx, projectIDs)
	if err != nil {
		return err
	}
	for i := range projects {
		projects[i].CompanyId = nil
		customer, hasCustomer := customers[projectIDs[i]]
		if edgesRead && !hasCustomer {
			continue
		}
		if customer != nil {
			id := openapi_types.UUID(customer.UUID)
			projects[i].CompanyId = &id
			continue
		}
		named := []string{filterCompanyIDOnProject}
		projects[i].MaskedFields = &named
	}
	return nil
}
