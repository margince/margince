// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefs

// Taking a snooze back.
//
// Setting an item aside was reversible only by waiting for it. A rep who
// snoozed the wrong row, or chose "until the meeting" when they meant "until
// tomorrow", had no way back — the toast offered an undo and there was no writer
// behind it, so the queue kept the row hidden until its condition happened to
// lift.
//
// This is the mirror of MarkSnoozed and deliberately not a general un-mark.
// `acted` and `dismissed` each mean a rep decided something about the deal, and
// reversing those is a different question with a different answer (dismissed
// already returns on new activity, acted on a material change). A snooze decides
// nothing except "not now", which is why it is the one mark that can simply be
// taken back.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// MarkUnsnoozed returns a set-aside item to the queue.
//
// `new` is the only state it can return to, and that is a fact about the writer
// rather than a choice made here: markItem admits a transition only from an
// actionable item, so an item that is snoozed was `new` when it was snoozed.
// Reading the prior state out of the audit trail would be a second source for
// something the state machine already fixes.
//
// A CONFLICT when the item is not snoozed, never a quiet success. There is
// nothing to take back, and answering 200 would tell a rep their click landed on
// a state that had already moved under them — the same reasoning markItem
// applies to a second mark.
//
// An item whose snooze condition has already lifted is still snoozed here. The
// row says so until a read re-surfaces it, the write is identical either way,
// and asking briefItemActionable would make the answer depend on whether a read
// had run rather than on what the rep did.
//
// No clock, unlike every mark beside it. Those stamp state_at; this clears it,
// because brief_item_state_stamped pairs a stamp with a decision and an item
// back in the queue carries none. The instant is not lost — the audit row keeps
// it — so there is nothing here for a caller's clock to decide.
func (e *BriefEngine) MarkUnsnoozed(ctx context.Context, itemID ids.UUID) (BriefRunItem, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return BriefRunItem{}, err
	}
	userID, err := briefUser(ctx)
	if err != nil {
		return BriefRunItem{}, err
	}

	var item BriefRunItem
	err = database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		owner, err := lockItemForMark(ctx, tx, itemID, &item)
		if err != nil {
			return err
		}
		if owner != userID {
			// Another rep's brief: existence-hiding, like every row-scope miss.
			return apperrors.ErrNotFound
		}
		if item.State != briefStateSnoozed {
			return apperrors.ErrConflict
		}

		// EVERY MARK COLUMN CLEARED, state_at included. brief_item_state_stamped
		// spells `(state = 'new') = (state_at IS NULL)`: an item in the queue has
		// never been marked, so a stamp beside `new` is a contradiction the
		// database refuses outright. Taking a snooze back therefore restores the
		// row a fresh item has rather than writing a new mark — which is what
		// "back in the queue" has to mean if the state is to keep its meaning.
		//
		// WHEN it was taken back is not lost with it: the audit row below carries
		// the instant, along with the snooze it replaced. The column says whether
		// the rep has decided anything about this item, and after an unsnooze
		// they have not.
		if _, err := tx.Exec(ctx, `
			UPDATE brief_item SET state = $2, state_at = NULL, snoozed_until = NULL,
			       reopen_on = NULL, reopen_ref = NULL WHERE id = $1`,
			itemID, briefStateNew); err != nil {
			return fmt.Errorf("brief: taking back the snooze on item %s: %w", itemID, err)
		}
		before := map[string]any{
			auditFieldState: item.State, auditFieldStateAt: item.StateAt,
			auditFieldSnoozedUntil: item.SnoozedUntil,
			auditFieldReopenOn:     item.ReopenOn, auditFieldReopenRef: item.ReopenRef,
		}
		after := map[string]any{
			auditFieldState: briefStateNew, auditFieldStateAt: nil,
			auditFieldSnoozedUntil: nil,
			auditFieldReopenOn:     nil, auditFieldReopenRef: nil,
		}
		if _, err := storekit.Audit(ctx, tx, "update", "brief_item", itemID, before, after); err != nil {
			return err
		}
		item.State = briefStateNew
		item.StateAt = nil
		item.SnoozedUntil = nil
		item.ReopenOn = ""
		item.ReopenRef = nil
		return nil
	})
	if err != nil {
		return BriefRunItem{}, err
	}
	return item, nil
}
