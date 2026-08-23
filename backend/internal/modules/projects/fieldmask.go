// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

// The project read mask, the sibling of fieldmask.go's deal mask and for the
// same reason: a projection that carries a reference to ANOTHER record must
// withhold the one its reader could not open, or the record becomes an
// existence oracle over rows that reader's own reads would refuse.
//
// A project has exactly one such reference — its anchor company. That company
// is capture-private until somebody promotes it (platform/auth rowscope.go),
// while the project hanging off it is read by every seat holding the object
// grant. So the two disagree by design, and this is where the wire answer is
// brought back in line with what the reader may actually see.

import (
	"context"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/platform/auth"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// filterOrganizationIDOnProject is masked_fields' name for the anchor
// reference. It is the wire member's own name, which is what a client reads
// the mask against.
const filterOrganizationIDOnProject = "organization_id"

// maskProjectForCaller applies the read mask to ONE row about to leave the
// store.
// It is a METHOD rather than a free function so every single-project answer
// carries the company list without each of the six call sites remembering to
// ask: a project read whose companies were missing looks exactly like a project
// with no companies, and there is no field a reader could check to tell.
func (s *Store) maskProjectForCaller(ctx context.Context, tx pgx.Tx, p crmcontracts.Project) (crmcontracts.Project, error) {
	one := []crmcontracts.Project{p}
	if err := maskProjects(ctx, tx, one); err != nil {
		return crmcontracts.Project{}, err
	}
	return s.withCompanies(ctx, tx, one[0])
}

// withCompanies fills a single project's company list from the edges, and
// derives organization_id from it: the customer edge is what that field has
// always meant, and deriving it here is what keeps the two from disagreeing.
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
func fillCompanies(
	ctx context.Context, tx pgx.Tx, p crmcontracts.Project, companies ProjectCompanies,
) (crmcontracts.Project, error) {
	on, err := companies(ctx, tx, ids.From[ids.ProjectKind](ids.UUID(p.Id)))
	if err != nil {
		return crmcontracts.Project{}, err
	}
	listed := make([]crmcontracts.ProjectCompany, 0, len(on))
	var customer *openapi_types.UUID
	for _, one := range on {
		listed = append(listed, crmcontracts.ProjectCompany{
			OrganizationId: openapi_types.UUID(one.OrganizationID.UUID),
			DisplayName:    one.DisplayName,
			Role:           one.Role,
		})
		if customer == nil && one.Role == CompanyRoleCustomer {
			id := openapi_types.UUID(one.OrganizationID.UUID)
			customer = &id
		}
	}
	p.Organizations = &listed
	p.OrganizationId = customer
	return p, nil
}

// maskProjects withholds, per row, the anchor company this reader may not
// open. ONE statement answers the whole page, never a probe per row.
func maskProjects(ctx context.Context, tx pgx.Tx, projects []crmcontracts.Project) error {
	orgIDs := make([]ids.UUID, 0, len(projects))
	for _, p := range projects {
		if p.OrganizationId != nil {
			orgIDs = append(orgIDs, ids.UUID(*p.OrganizationId))
		}
	}
	// VisibleSubset answers an empty list without a round trip, and it checks
	// the organization object grant as well as the row scope — a seat holding
	// no `organization.read` learns no company id from a project either.
	visible, err := auth.VisibleSubset(ctx, tx, "organization", orgIDs)
	if err != nil {
		return err
	}
	for i := range projects {
		anchor := projects[i].OrganizationId
		if anchor == nil || visible[ids.UUID(*anchor)] {
			continue
		}
		projects[i].OrganizationId = nil
		named := []string{filterOrganizationIDOnProject}
		projects[i].MaskedFields = &named
	}
	return nil
}
