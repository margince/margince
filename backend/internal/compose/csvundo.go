// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Undoing a csv import: archiving what one landed, through each object's own
// archive path. Kept apart from the landing itself because reversing a run is
// its own act — it runs on a later request, on its own approval, and it must be
// idempotent in a way the landing is not.

var _ migration.UndoWriters = (*csvWriters)(nil)

// Reverse archives the native row a csv import created (IEM-WIRE-9), through
// each object's existing archive path — soft-delete only, per the contract's
// own convention, never a hard delete. Idempotent: a resumed undo may replay
// a row whose archive committed but whose checkpoint advance did not, and an
// already-archived row is left exactly as it is rather than re-archived (or
// erroring on a live-only read that no longer finds it).
func (w *csvWriters) Reverse(
	ctx context.Context, object string, nativeID ids.UUID, importedAt time.Time,
) error {
	// The precondition each archive re-asks under its own row lock. The undo's
	// page-level check already skipped the rows it found touched; this is what
	// puts the question in the same transaction as the write, since that check
	// and this write are different transactions and a human can act between them.
	untouched := contacts.NotTouchedByHumanSince(importedAt)
	archived, err := w.isArchived(ctx, object, nativeID)
	if err != nil {
		return fmt.Errorf("import undo: reading %s %s: %w", object, nativeID, err)
	}
	if archived {
		return nil
	}
	switch object {
	case migration.ObjectLead:
		_, err = w.contacts.DisqualifyLead(ctx, ids.From[ids.LeadKind](nativeID), contacts.DisqualifyLeadInput{}, untouched)
	case migration.ObjectCompany:
		_, err = w.contacts.ArchiveCompany(ctx, ids.From[ids.CompanyKind](nativeID), nil, untouched)
	case migration.ObjectContact:
		// The archive cascades to the contact's addresses, numbers and
		// relationships, so the child rows this run created go with it.
		_, err = w.contacts.ArchiveContact(ctx, ids.From[ids.ContactKind](nativeID), nil, untouched)
	default:
		return fmt.Errorf("import undo: %q is not a reversible object: %w", object, apperrors.ErrConflict)
	}
	if err != nil {
		return fmt.Errorf("import undo: reversing %s %s: %w", object, nativeID, err)
	}
	return nil
}
