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
// label.
//
// It is NOT atomic with the narrowing, and nothing here can make it so. Every
// caller is an async consumer reacting to the activity.updated event SetAudience
// emitted AFTER committing, so between the narrowing and this pass there is a
// real interval in which the row is limited and its vector and label still
// stand.
// Held by: TestTheRetractionRunsOnlyBehindACommittedNarrowing
// (backend/gates/audienceretractioncallers_test.go) The interval is the bus's latency and
// the retry that follows a failure; it is not a window the retraction closes,
// it is one it ENDS.
//
// What stops the interval from mattering is on the write side, not here: the
// embedding upsert and SetCaptureLabel both re-test the audience in their own
// statements, so nothing new is derived from a limited message while this pass
// is still on its way. This collects what was derived before.
//
// It is deliberately not the inverse of "everything derived from this message".
// The derived SIGNALS are narrowed rather than deleted — signals carry their own
// visibility and a narrowed signal is still the owner's — and that omission is a
// boundary rather than a miss.
//
// The profile fields a signature enrichment wrote ARE retracted, through the
// seam below. Only the ones nobody has taken over: a value a person restored or
// corrected stays, and RetractSignatureFieldsTx states which columns tell those
// apart.
//
// Idempotent: re-running deletes nothing more, which is what lets the consumer
// that calls it retry.
// SignatureFieldRetractor removes the profile fields one message's signature
// wrote. people owns that table, so compose injects the edge, and it is required
// rather than optional — a narrowing that skipped it would leave the message's
// content on a record everybody reads.
type SignatureFieldRetractor func(ctx context.Context, tx pgx.Tx, activityID ids.UUID) error

// RetractDerivedForActivityTx collects what was derived from a message that has
// just been narrowed, as described above.
func RetractDerivedForActivityTx(
	ctx context.Context, tx pgx.Tx, activityID ids.UUID, retractFields SignatureFieldRetractor,
) error {
	// THE ACTIVITY FIRST, BEFORE ANY DERIVED ROW. Both things this function
	// removes have a concurrent writer that takes the activity and then the
	// derived row — the embedding upsert holds a share lock on the activity
	// while it writes the vector, the classifier's SetCaptureLabel updates the
	// activity itself — so taking them in the other order here would deadlock
	// against both, and a deadlock in the consumer is a narrowing whose
	// residue is never collected.
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
	// The lock above is what makes this stick against the classifier: it read
	// the backlog before the narrowing committed, and without the lock its
	// write can land after this clear, leaving the narrowed message labelled
	// from text nobody may read.
	if _, err := tx.Exec(ctx, `
		UPDATE activity SET capture_label = NULL WHERE id = $1 AND capture_label IS NOT NULL`, activityID); err != nil {
		return fmt.Errorf("activities: clearing the narrowed activity's attention label: %w", err)
	}
	// The profile fields this message's signature wrote, last and inside the
	// same lock. A title or a phone number lifted from a signature block is the
	// message's content restated on a record everybody can see, so a narrowing
	// that left them behind would limit the message and publish what it said.
	//
	return retractFields(ctx, tx, activityID)
}
