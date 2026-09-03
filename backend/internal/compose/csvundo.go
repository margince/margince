// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
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
func (w *csvWriters) Reverse(ctx context.Context, object string, nativeID ids.UUID) error {
	switch object {
	case migration.ObjectLead:
		lead, err := w.people.GetLead(ctx, ids.From[ids.LeadKind](nativeID), storekit.IncludeArchived)
		if err != nil {
			return fmt.Errorf("import undo: reading lead %s: %w", nativeID, err)
		}
		if lead.ArchivedAt != nil {
			return nil
		}
		if _, err := w.people.DisqualifyLead(ctx, ids.From[ids.LeadKind](nativeID), people.DisqualifyLeadInput{}); err != nil {
			return fmt.Errorf("import undo: reversing lead %s: %w", nativeID, err)
		}
		return nil
	case migration.ObjectOrganization:
		org, err := w.people.GetOrganization(ctx, ids.From[ids.OrganizationKind](nativeID), storekit.IncludeArchived)
		if err != nil {
			return fmt.Errorf("import undo: reading organization %s: %w", nativeID, err)
		}
		if org.ArchivedAt != nil {
			return nil
		}
		if _, err := w.people.ArchiveOrganization(ctx, ids.From[ids.OrganizationKind](nativeID), nil); err != nil {
			return fmt.Errorf("import undo: reversing organization %s: %w", nativeID, err)
		}
		return nil
	case migration.ObjectPerson:
		person, err := w.people.GetPerson(ctx, ids.From[ids.PersonKind](nativeID), storekit.IncludeArchived)
		if err != nil {
			return fmt.Errorf("import undo: reading person %s: %w", nativeID, err)
		}
		if person.ArchivedAt != nil {
			return nil
		}
		// The archive cascades to person_email, person_phone and the person's
		// relationships, so the child rows this run created go with it.
		if _, err := w.people.ArchivePerson(ctx, ids.From[ids.PersonKind](nativeID), nil); err != nil {
			return fmt.Errorf("import undo: reversing person %s: %w", nativeID, err)
		}
		return nil
	default:
		return fmt.Errorf("import undo: %q is not a reversible object: %w", object, apperrors.ErrConflict)
	}
}
