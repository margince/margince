// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The adapter that lets the finance card name a customer's billing contacts
// without finance importing contacts. Same shape as signalStrength next door:
// the sibling read is bound HERE, which is the one place two modules may meet.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/company360"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// financeBillingContacts answers finance.BillingContactReader from the
// contacts store that owns the relationship table.
//
// It projects through company360.BillingContactPayload rather than mapping the
// rows again, so the finance card and the company page render one list built
// one way. Two projections of "who gets the invoice" would eventually disagree,
// and the reader has no way to tell which panel is the stale one.
type financeBillingContacts struct{ contacts *contacts.Store }

func (f financeBillingContacts) BillingContactsFor(
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID,
) ([]crmcontracts.BillingContact, error) {
	rows, err := f.contacts.BillingContactsFor(ctx, tx, companyID)
	if err != nil {
		return nil, err
	}
	return company360.BillingContactPayload(rows), nil
}
