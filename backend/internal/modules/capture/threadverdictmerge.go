// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// A seat's thread verdict when two threads merge into one.
//
// The verdict is per seat and per thread, and it is what a LATER message of
// the thread inherits (verdictinherit.go). The messages already stored carry
// their own verdict on their capture_import row, so nothing here changes who
// can read an existing message. What it decides is how the merged conversation
// treats the next message — and that must never be more open than either half
// was.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// verdictHoldRank orders the verdict states by how firmly they hold a thread.
// A holding answer outranks any opening one, and the owner's own hold outranks
// a classifier's.
func verdictHoldRank(status string) int {
	switch status {
	case VerdictHeldByOwner:
		return 5
	case VerdictHeld:
		return 4
	case VerdictUnsure:
		return 3
	case VerdictPending:
		return 2
	default: // cleared, shared_by_owner: opening answers
		return 1
	}
}

// mergedVerdict says what one seat's merged thread answers, given the verdict
// each half carried: keepFrom reports that the `from` half's row survives (else
// the `to` half's), and reopen that the survivor goes back to pending.
//
// A holding verdict wins whichever half it came from. A pending one wins over
// an opening one: part of the conversation is still unjudged. Two opening
// answers were each given about half a conversation, so the whole goes back to
// the classifier rather than inheriting an answer nobody gave about it.
func mergedVerdict(fromStatus, toStatus string) (keepFrom, reopen bool) {
	fromRank, toRank := verdictHoldRank(fromStatus), verdictHoldRank(toStatus)
	switch {
	case fromRank > toRank:
		return true, false
	case fromRank < toRank:
		return false, false
	case fromRank == 1:
		return false, true
	default:
		return false, false
	}
}

// ThreadVerdictMergeTx folds every seat's verdict on thread `from` into thread
// `to`. A seat with a verdict on only one half keeps it as it stands: that
// answer was given about the messages this seat holds, and the other half holds
// none of theirs.
func ThreadVerdictMergeTx(ctx context.Context, tx pgx.Tx, from, to string) error {
	if from == "" || to == "" || from == to {
		return nil
	}
	rows, err := tx.Query(ctx, `
		SELECT f.id, f.status, t.id, t.status
		  FROM capture_thread_verdict f
		  JOIN capture_thread_verdict t ON t.user_id = f.user_id AND t.thread_key = $2
		 WHERE f.thread_key = $1
		 ORDER BY f.id
		   FOR UPDATE`, from, to)
	if err != nil {
		return fmt.Errorf("capture: reading the verdicts two threads carry: %w", err)
	}
	type pair struct {
		fromID, toID         ids.UUID
		fromStatus, toStatus string
	}
	pairs, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (pair, error) {
		var p pair
		err := r.Scan(&p.fromID, &p.fromStatus, &p.toID, &p.toStatus)
		return p, err
	})
	if err != nil {
		return fmt.Errorf("capture: reading the verdicts two threads carry: %w", err)
	}
	for _, p := range pairs {
		keepFrom, reopen := mergedVerdict(p.fromStatus, p.toStatus)
		drop := p.fromID
		if keepFrom {
			drop = p.toID
		}
		if _, err := tx.Exec(ctx, `DELETE FROM capture_thread_verdict WHERE id = $1`, drop); err != nil {
			return fmt.Errorf("capture: retiring the weaker of two thread verdicts: %w", err)
		}
		if reopen {
			// The same reset reopenClearedThreadTx makes, and for its reason:
			// the classifier must not be shown the message a previous answer
			// was about and have its new answer applied to the whole. The
			// question is asked about this seat's newest message of the
			// merged conversation instead; a claim with no message to read
			// retires rather than judging an empty prompt.
			if _, err := tx.Exec(ctx, `
				UPDATE capture_thread_verdict v
				   SET status = 'pending', kind = NULL, confidence = NULL,
				       first_activity_id = (
				           SELECT ci.activity_id
				             FROM capture_import ci
				             JOIN activity a ON a.id = ci.activity_id
				            WHERE ci.user_id = v.user_id
				              AND a.thread_key IN ($2, $3) AND a.archived_at IS NULL
				            ORDER BY a.occurred_at DESC, a.id DESC
				            LIMIT 1),
				       seen_addresses = '{}',
				       resolved_at = NULL, next_attempt_at = now(), updated_at = now()
				 WHERE v.id = $1`, p.toID, from, to); err != nil {
				return fmt.Errorf("capture: re-opening a merged thread's verdict: %w", err)
			}
		}
	}
	// Every verdict still on `from` is either the survivor of a pair or a seat's
	// only answer; both now belong to `to`.
	if _, err := tx.Exec(ctx, `
		UPDATE capture_thread_verdict SET thread_key = $2, updated_at = now()
		 WHERE thread_key = $1`, from, to); err != nil {
		return fmt.Errorf("capture: moving thread verdicts to the thread they joined: %w", err)
	}
	return nil
}
