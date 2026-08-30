// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// What a message's content produced while it was workspace-visible has to go
// when the message stops being workspace-visible. Narrowing the row changes who
// may read the row; it does not reach the vector a similarity probe answers
// from, nor the attention label that put the message on a colleague's screen.
// Both are the message's own text in another shape, and both are readable by
// people the narrowing just excluded.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RetractDerivedForActivityTx drops what one activity's own text produced for
// readers who no longer belong to its audience: its embedding and its attention
// label. It runs on the CALLER's transaction, so the narrowing and the
// retraction commit together — a narrowing that committed without its retraction
// would leave the residue behind with nothing scheduled to collect it.
//
// It is deliberately not the inverse of "everything derived from this message".
// The derived SIGNALS are narrowed rather than deleted (signals carry their own
// visibility and a narrowed signal is still the owner's), and the profile fields
// a signature enrichment wrote are a separate question with a human-edit
// conflict to settle. Those two are named here so the next reader knows the
// omission is a boundary and not a miss.
//
// Idempotent: re-running deletes nothing more, which is what lets the consumer
// that calls it retry.
func RetractDerivedForActivityTx(ctx context.Context, tx pgx.Tx, activityID ids.UUID) error {
	// The vector is the sharper of the two. It is built as the system principal
	// over the whole workspace and queried by everyone, so a stale vector for a
	// narrowed row does not merely return the row — it returns whoever's search
	// phrase was semantically near a colleague's held mail.
	if _, err := tx.Exec(ctx, `
		DELETE FROM embedding WHERE entity_type = 'activity' AND entity_id = $1`, activityID); err != nil {
		return fmt.Errorf("activities: dropping the narrowed activity's embedding: %w", err)
	}
	// The label is the message's classified attention state — "reply needed",
	// "waiting on them" — derived from its subject and body and shown on a
	// shared worklist. Clearing it rather than recomputing it is correct: the
	// backlog predicate now excludes the row, so no pass will recompute it, and
	// a label with no readable message behind it is the residue itself.
	//
	// The row is locked first because the label has a concurrent writer: the
	// classifier's SetCaptureLabel is a separate transaction that read the
	// backlog before this narrowing committed, and without the lock its write
	// can land after the clear — leaving the narrowed message labelled from
	// text nobody may read. The lock makes the two serialise, and the
	// classifier's own write then sees the narrowed row.
	//
	// An archived row is locked too (IncludeArchived): a narrowing that arrives
	// after an archive still owes the retraction, and refusing here would leave
	// the residue behind exactly when the message is least likely to be looked
	// at again.
	if _, err := storekit.LockRow(ctx, tx, "activity", activityID, storekit.IncludeArchived); err != nil {
		// A row erased between the event and this pass has no residue to
		// retract; the destruction path already took it.
		if errors.Is(err, apperrors.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("activities: locking the narrowed activity: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE activity SET capture_label = NULL WHERE id = $1 AND capture_label IS NOT NULL`, activityID); err != nil {
		return fmt.Errorf("activities: clearing the narrowed activity's attention label: %w", err)
	}
	return nil
}
