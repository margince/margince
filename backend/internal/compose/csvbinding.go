// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// isArchived reads the bound record with archived rows included, so a row-scope
// miss still answers not-found rather than reading as archived.
func (w *csvWriters) isArchived(ctx context.Context, id ids.UUID) (bool, error) {
	switch w.object {
	case migration.ObjectLead:
		lead, err := w.contacts.GetLead(ctx, ids.From[ids.LeadKind](id), storekit.IncludeArchived)
		return err == nil && lead.ArchivedAt != nil, err
	case migration.ObjectCompany:
		company, err := w.contacts.GetCompany(ctx, ids.From[ids.CompanyKind](id), storekit.IncludeArchived)
		return err == nil && company.ArchivedAt != nil, err
	case migration.ObjectContact:
		contact, err := w.contacts.GetContact(ctx, ids.From[ids.ContactKind](id), storekit.IncludeArchived)
		return err == nil && contact.ArchivedAt != nil, err
	default:
		return false, fmt.Errorf("import: %q is not an importable object", w.object)
	}
}

// lookup resolves an external id to the record an earlier run landed for it.
func (w *csvWriters) lookup(ctx context.Context, object, externalID string) (ids.UUID, bool, error) {
	if object != w.object {
		return ids.UUID{}, false, fmt.Errorf("import: this run carries %q, not %q", w.object, object)
	}
	if id, ok := w.nativeIDs[externalID]; ok {
		return id, true, nil
	}
	id, found, err := w.identities.LookupIdentity(ctx, csvSourceSystem(), object, externalID)
	if err != nil {
		return ids.UUID{}, false, err
	}
	if !found {
		return id, false, nil
	}
	// A binding to an archived record names nothing a re-import can update, so
	// the key counts as unlanded.
	archived, err := w.isArchived(ctx, id)
	if err != nil {
		return ids.UUID{}, false, err
	}
	if archived {
		w.archivedBindings[externalID] = true
		return ids.UUID{}, false, nil
	}
	w.nativeIDs[externalID] = id
	return id, true, nil
}
