// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// isArchived reads a record of the given object with archived rows included, so
// a row-scope miss still answers not-found rather than reading as archived.
func (w *csvWriters) isArchived(ctx context.Context, object string, id ids.UUID) (bool, error) {
	switch object {
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
		return false, fmt.Errorf("import: %q is not an importable object", object)
	}
}

// errBoundRecordArchived reports a key whose record was archived by anything
// other than a finished undo of the run that landed it.
var errBoundRecordArchived = errors.New("the record this row landed as is archived")

// boundArchivedReason is what a row is told when its record is archived.
const boundArchivedReason = "this row's record was archived, so it is not updated; restore the record to update it"

// lookup resolves an external id to the record an earlier run landed for it.
//
// A key whose run finished its undo, and whose record is archived, is free
// again. Any other archived record keeps the key. A paused undo still pages its
// rows, and a record archived by hand must not come back as a twin.
func (w *csvWriters) lookup(ctx context.Context, object, externalID string) (ids.UUID, bool, error) {
	if object != w.object {
		return ids.UUID{}, false, fmt.Errorf("import: this run carries %q, not %q", w.object, object)
	}
	if id, ok := w.nativeIDs[externalID]; ok {
		return id, true, nil
	}
	binding, found, err := w.identities.LookupBinding(ctx, csvSourceSystem(), object, externalID)
	if err != nil || !found {
		return ids.UUID{}, false, err
	}
	archived, err := w.isArchived(ctx, object, binding.NativeID)
	if err != nil {
		return ids.UUID{}, false, err
	}
	switch {
	case !archived:
		w.nativeIDs[externalID] = binding.NativeID
		return binding.NativeID, true, nil
	case binding.RunStatus == migration.StatusUndone:
		return ids.UUID{}, false, nil
	default:
		return binding.NativeID, true, errBoundRecordArchived
	}
}
