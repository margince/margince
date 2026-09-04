// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The owner publishing their own captured contact.
//
// Capture privacy is the importing user's, not a tier of row scope: an admin
// reading a colleague's unpromoted captured contacts is precisely the
// disclosure the boundary exists to prevent. The other side of that is that the
// owner CAN decide, and until now they had no way to say so — the only route
// out of `owner` was a classifier verdict, and a contact the ceiling never
// asked about, or that a model judged an advisor, had none at all.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PromoteOwnCapturedPerson publishes a captured contact to the workspace, on
// the authority of the seat whose mailbox it came from.
//
// The owner is taken from the AUTHENTICATED principal and written into the
// statement, never accepted from the caller. That is what ties the human's
// authorisation to the row being disclosed: a request naming somebody else's
// private contact matches nothing, and a promotion by email match — the shape
// the verdict path uses, under a system principal that bypasses both object
// RBAC and capture privacy — would have published a colleague's contact to
// anybody who could reach this door.
//
// A miss is ErrNotFound, not a permission error: a colleague's capture-private
// contact must not be distinguishable from one that does not exist.
func (s *Store) PromoteOwnCapturedPerson(ctx context.Context, id ids.PersonID) error {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID == ids.Nil {
		// The mailbox owner is a person. A connector or the verdict engine
		// reaches the workspace through their own path, which is judged rather
		// than asserted.
		return apperrors.ErrPermissionDenied
	}
	if !actor.SeatType.CanMutate() {
		return apperrors.ErrPermissionDenied
	}
	// The object grant, on the same terms as any other person write. The row
	// half is NOT auth.EnsureWritable: that answers "may this caller edit this
	// record", which a write grant or a team scope can satisfy — and neither is
	// authority to publish somebody's private correspondence. Capture privacy
	// is the importing user's alone, so the row test is ownership, and it lives
	// in the statement below where it cannot be skipped.
	if err := auth.Require(ctx, entityPerson, principal.ActionUpdate); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE person SET visibility = $3
			 WHERE id = $1 AND owner_id = $2
			   AND visibility = $4 AND archived_at IS NULL`,
			id.UUID, actor.UserID, visibilityWorkspace, visibilityOwner)
		if err != nil {
			return fmt.Errorf("people: publishing a captured contact: %w", err)
		}
		if tag.RowsAffected() == 0 {
			// Not theirs, not private, or not there. All three answer the same
			// way: existence is what the boundary hides.
			return apperrors.ErrNotFound
		}
		auditID, err := storekit.Audit(ctx, tx, "update", entityPerson, id.UUID,
			map[string]any{fieldVisibility: visibilityOwner},
			map[string]any{fieldVisibility: visibilityWorkspace})
		if err != nil {
			return fmt.Errorf("people: recording a captured contact's publication: %w", err)
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID,
			crmcontracts.PublicEventPersonUpdated{
				ChangedFields: map[string]any{fieldVisibility: visibilityWorkspace},
			}); err != nil {
			return err
		}
		// The mail and meetings that were only ever linked to this contact
		// follow it. Without this the record is visible and its history is not,
		// which reads to a colleague as a contact nobody has ever spoken to.
		_, err = s.PromotePersonCohortTx(ctx, tx, id)
		return err
	})
}
