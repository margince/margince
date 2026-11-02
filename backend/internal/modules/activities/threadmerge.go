// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Native reply edges remain with their source so late parents can be resolved.
// Grouping and sales-state changes share their owning transaction.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RecordMailReferencesTx stores the Message-IDs one email says it answers.
//
// Idempotent: a re-sync of the same message states the same links. The ids are
// the sender's text; capture acts on an edge only after proving the same
// importing seat holds both ends.
func RecordMailReferencesTx(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, referenced []string) error {
	if len(referenced) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_mail_reference (activity_id, referenced_id, reference_order)
		SELECT @id, r, position::integer FROM unnest(@references::text[]) WITH ORDINALITY AS refs(r,position)
		ON CONFLICT (activity_id, referenced_id) DO UPDATE SET reference_order=
          CASE WHEN activity_mail_reference.reference_order=0 THEN EXCLUDED.reference_order
          ELSE least(activity_mail_reference.reference_order,EXCLUDED.reference_order) END
        WHERE activity_mail_reference.reference_order=0 OR EXCLUDED.reference_order<activity_mail_reference.reference_order`,
		pgx.NamedArgs{"id": activityID, "references": referenced}); err != nil {
		return fmt.Errorf("activities: recording what a message replies to: %w", err)
	}
	return nil
}

// ThreadMergeTx moves every message of thread `from` into thread `to`, and the
// thread-scoped "not sales" judgement with it.
//
// The judgement survives only where BOTH threads carried it. It hides a thread
// from the worklist, so carrying one half's judgement onto the other half would
// take messages nobody judged off the list without a word. Dropping it puts
// them back, where a rep sees them and can judge again.
//
// Archived rows move too: a thread key is a grouping, and an erased or
// restricted message still belongs to the conversation it was in.
func ThreadMergeTx(ctx context.Context, tx pgx.Tx, from, to string) error {
	if from == "" || to == "" || from == to {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE activity SET thread_key = $2
		 WHERE thread_key = $1`, from, to); err != nil {
		return fmt.Errorf("activities: moving a thread's messages into the one it joins: %w", err)
	}
	// Both halves' judgements, kept only where they agree.
	if _, err := tx.Exec(ctx, `
		DELETE FROM activity_sales_state s
		 WHERE s.thread_key IN ($1, $2)
		   AND NOT EXISTS (SELECT 1 FROM activity_sales_state o
		                    WHERE o.thread_key = CASE WHEN s.thread_key = $1 THEN $2 ELSE $1 END
		                      AND o.kind = s.kind AND o.channel_provider = s.channel_provider)`,
		from, to); err != nil {
		return fmt.Errorf("activities: reconciling the merged threads' judgements: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM activity_sales_state WHERE thread_key = $1`, from); err != nil {
		return fmt.Errorf("activities: retiring the merged thread's judgements: %w", err)
	}
	return nil
}
