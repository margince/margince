// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

// The billing-contacts section: who handles this account's invoices, and in
// what capacity.
//
// The read itself belongs to the contacts module, which owns the relationship
// table. This file is the projection into the page's shape — and the same
// contacts read serves the finance card, so the two panels can never come to
// disagree about who the recipient is.

import (
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
)

// readBillingContacts fills the section, or refuses with
// ErrPermissionDenied so the loop names it in sections_omitted.
//
// Empty is a REAL answer and is rendered as one: a paying customer with nobody
// named as recipient is a gap the finance tab should show, not a blank section.
// That is why the slice is always non-nil on success — an absent field means
// "you may not see this", and the two must not collapse into one rendering.
func (a *assembly) readBillingContacts() error {
	rows, err := a.svc.contacts.BillingContactsFor(a.ctx, a.tx, a.companyID)
	if err != nil {
		return err
	}
	payload := BillingContactPayload(rows)
	a.out.BillingContacts = &payload
	return nil
}

// BillingContactPayload turns the store's rows into the wire shape. Exported
// because the finance card renders the identical list from the identical read,
// and two spellings of one projection would be two answers to "who gets the
// invoice" — the half that drifted being the one a reader acts on.
func BillingContactPayload(rows []contacts.BillingContact) []crmcontracts.BillingContact {
	out := make([]crmcontracts.BillingContact, 0, len(rows))
	for _, r := range rows {
		out = append(out, crmcontracts.BillingContact{
			RelationshipId: openapi_types.UUID(r.RelationshipID),
			ContactId:      openapi_types.UUID(r.ContactID.UUID),
			FullName:       r.FullName,
			Role:           crmcontracts.BillingContactRole(r.Role),
			Title:          r.Title,
			Email:          r.Email,
		})
	}
	return out
}
