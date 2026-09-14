// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

// The finance card names who handles the customer's invoices.
//
// That fact lives in the relationship table, which the contacts module owns,
// and a module never reads a sibling's tables. So the read arrives as a port
// the composition layer binds — the narrowest one that answers the question,
// taking the transaction the card is already assembling in, so the names are
// consistent with the figures beside them.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// BillingContactReader answers who handles one company's invoices.
//
// It returns the wire shape rather than a finance-owned struct: the company
// page renders the identical list from the identical read, and a second
// projection here would be a second answer to "who gets the invoice" — with the
// half that drifted being the one somebody acts on.
type BillingContactReader interface {
	BillingContactsFor(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID) ([]crmcontracts.BillingContact, error)
}

// WithBillingContacts binds the reader. Optional by construction: an
// installation that has not wired it serves the card without the panel rather
// than failing, exactly as it serves the card with no accounting connection.
func (s *Store) WithBillingContacts(reader BillingContactReader) *Store {
	s.billingContacts = reader
	return s
}

// WithBillingContacts wires the reader through the transport, the way compose
// reaches every other injected port.
func (h Handlers) WithBillingContacts(reader BillingContactReader) Handlers {
	h.store = h.store.WithBillingContacts(reader)
	return h
}

// readBillingContacts fills the card's panel, and leaves it ABSENT when the
// caller may not see it.
//
// A refusal is not an error here. A reader with the finance grant but no
// contact grant is entitled to the figures, and failing the whole card over the
// panel would take the money away to protect the names. Absent says "not for
// you"; an empty array says "nobody is named", and the card renders those
// differently.
func (s *Store) readBillingContacts(
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, out *crmcontracts.CompanyFinanceSummary,
) error {
	if s.billingContacts == nil {
		return nil
	}
	rows, err := s.billingContacts.BillingContactsFor(ctx, tx, companyID)
	if err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return nil
		}
		return err
	}
	out.BillingContacts = &rows
	return nil
}
