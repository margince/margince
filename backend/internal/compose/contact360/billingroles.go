// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

// The billing-roles section: which companies this contact handles invoices for.
//
// The same edge the company page reads, asked from the other end. A contact is
// often a billing contact at one customer and nothing at the next, so the list
// is short and belongs beside their employers rather than inside them: where
// somebody WORKS and whose invoices they HANDLE are different facts, and an
// external accountant holds the second without the first.

import (
	"context"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// billingRolesSection fills the section, or refuses with ErrPermissionDenied so
// the assembly names it in sections_omitted.
//
// Empty is a real answer: most contacts handle nobody's invoices, and an empty
// list says so. Absent means the caller lacks the grant, which is why the slice
// is always non-nil on success.
func (s *Service) billingRolesSection(
	ctx context.Context, tx pgx.Tx, contactID ids.ContactID, out *crmcontracts.Contact360,
) error {
	rows, err := s.contacts.BillingRolesOf(ctx, tx, contactID)
	if err != nil {
		return err
	}
	payload := make([]crmcontracts.BillingCompany, 0, len(rows))
	for _, r := range rows {
		payload = append(payload, crmcontracts.BillingCompany{
			RelationshipId: openapi_types.UUID(r.RelationshipID),
			CompanyId:      openapi_types.UUID(r.CompanyID.UUID),
			CompanyName:    r.CompanyName,
			Role:           crmcontracts.BillingContactRole(r.Role),
		})
	}
	out.BillingRoles = &payload
	return nil
}
