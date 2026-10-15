// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// read answers the stored record as its own JSON: the surface changedFields
// compares against, and the shape the contract serves for it.
func (w *csvWriters) read(ctx context.Context, id ids.UUID) ([]byte, error) {
	switch w.object {
	case migration.ObjectLead:
		lead, err := w.contacts.GetLead(ctx, ids.From[ids.LeadKind](id), storekit.LiveOnly)
		if err != nil {
			return nil, err
		}
		return encodeRecord(lead)
	case migration.ObjectCompany:
		company, err := w.contacts.GetCompany(ctx, ids.From[ids.CompanyKind](id), storekit.LiveOnly)
		if err != nil {
			return nil, err
		}
		return encodeRecord(company)
	case migration.ObjectContact:
		contact, err := w.contacts.GetContact(ctx, ids.From[ids.ContactKind](id), storekit.LiveOnly)
		if err != nil {
			return nil, err
		}
		return encodeRecord(contact)
	case migration.ObjectDeal:
		deal, err := w.deals.GetDeal(ctx, ids.From[ids.DealKind](id), storekit.LiveOnly)
		if err != nil {
			return nil, err
		}
		return encodeRecord(deal)
	default:
		return nil, fmt.Errorf("import: %q is not an importable object", w.object)
	}
}

// encodeRecord renders one stored record as JSON. Generic so no wire type
// is widened to an empty interface on the way through.
func encodeRecord[T crmcontracts.Lead | crmcontracts.Company | crmcontracts.Contact | crmcontracts.Deal](record T) ([]byte, error) {
	encoded, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("import: reading the stored record: %w", err)
	}
	return encoded, nil
}
