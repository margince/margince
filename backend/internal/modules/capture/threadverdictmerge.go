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
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
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

// ErrVerdictsKeepArriving is the merge giving up, told apart from a database
// failure by its callers and by the cases that cover both.
var ErrVerdictsKeepArriving = errors.New("verdicts keep arriving for seats mid-merge")

// mergeVerdictPasses bounds the re-pairing below.
//
// A pass resolves every pair it can see and moves what has no counterpart. A
// second pass is needed only when a verdict arrived for a seat mid-merge.
// Three is slack rather than an expectation. Needing more means verdicts
// arrive faster than a merge folds them, which is a load question.
const mergeVerdictPasses = 3

// ThreadVerdictMergeTx folds every seat's verdict on thread `from` into thread
// `to`. A seat with a verdict on only one half keeps it as it stands: that
// answer was given about the messages this seat holds, and the other half holds
// none of theirs.
//
// Merges are serialized against each other by one advisory lock, and verdict
// creation does not take it. So a seat holding a verdict on `from` alone can
// gain one on `to` (threadverdict.go) between the pair read and the move.
//
// The move leaves that row where it is rather than colliding with
// `capture_thread_verdict_thread_user_key`. The next pass folds it by the rank
// rule, like any other pair.
func ThreadVerdictMergeTx(ctx context.Context, tx pgx.Tx, from, to string) error {
	if from == "" || to == "" || from == to {
		return nil
	}
	for pass := 1; pass <= mergeVerdictPasses; pass++ {
		if err := foldVerdictPairs(ctx, tx, from, to); err != nil {
			return err
		}
		left, err := moveUnpairedVerdicts(ctx, tx, from, to)
		if err != nil {
			return err
		}
		if left == 0 {
			return nil
		}
	}
	// Failing is the right end here. The verdicts still on `from` belong to a
	// thread key nothing reads, so reporting success would hide them.
	return fmt.Errorf("capture: folding thread verdicts into the thread they joined after %d passes: %w",
		mergeVerdictPasses, ErrVerdictsKeepArriving)
}

// foldVerdictPairs retires the weaker verdict of every seat holding one on
// both halves, and reopens the survivor where neither half judged the whole.
func foldVerdictPairs(ctx context.Context, tx pgx.Tx, from, to string) error {
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
		if !reopen {
			continue
		}
		if err := reopenMergedVerdict(ctx, tx, p.toID, from, to); err != nil {
			return err
		}
	}
	return nil
}

// reopenMergedVerdict sends a merged thread's verdict back to the classifier.
//
// The same reset reopenClearedThreadTx makes, and for its reason. The
// classifier must not be shown the message a previous answer was about, then
// have its new answer applied to the whole.
//
// The question is asked about this seat's newest message of the merged
// conversation instead.
//
// That message is one the classifier may read, without a predicate saying so.
// `threadOwedAnswerSQL` requires `restricted_at IS NULL` on
// first_activity_id, and a restricted activity is always archived, held by
// `activity_restricted_is_archived`. So the `archived_at IS NULL` above
// already excludes every restricted row, and adding a restriction filter here
// would be a predicate that cannot change the result.
//
// A claim with no message to read retires rather than judging an empty
// prompt.
func reopenMergedVerdict(ctx context.Context, tx pgx.Tx, verdictID ids.UUID, from, to string) error {
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
		 WHERE v.id = $1`, verdictID, from, to); err != nil {
		return fmt.Errorf("capture: re-opening a merged thread's verdict: %w", err)
	}
	return nil
}

// moveUnpairedVerdicts moves each verdict whose seat holds none on `to`, and
// reports how many it left behind.
//
// A row is left where it is when its seat gained a verdict on `to` since the
// pair read. Moving it would violate the one-verdict-per-seat-per-thread index,
// and choosing a winner is foldVerdictPairs's job, not this statement's.
func moveUnpairedVerdicts(ctx context.Context, tx pgx.Tx, from, to string) (int, error) {
	// A savepoint, because the guard narrows the window and does not close it.
	//
	// `NOT EXISTS` is answered from the statement's own snapshot. A verdict
	// committed after that snapshot is invisible to it, while the unique index
	// still refuses the row.
	//
	// So a refused move is rolled back and its rows are left for the next
	// pass, which costs a re-run of the moves. Letting the error out instead
	// would fail the capture.
	move, err := tx.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("capture: opening the savepoint the verdict move needs: %w", err)
	}
	_, moveErr := move.Exec(ctx, `
		UPDATE capture_thread_verdict v
		   SET thread_key = $2, updated_at = now()
		 WHERE v.thread_key = $1
		   AND NOT EXISTS (
		       SELECT 1 FROM capture_thread_verdict t
		        WHERE t.thread_key = $2 AND t.user_id = v.user_id)`, from, to)
	if moveErr == nil {
		// Releasing the savepoint is part of the move. A commit that fails
		// leaves the same rows unmoved, so it takes the same arm.
		moveErr = move.Commit(ctx)
	}
	if moveErr != nil {
		rollbackErr := move.Rollback(ctx)
		constraint, duplicate := storekit.UniqueViolation(moveErr)
		if rollbackErr != nil || !duplicate ||
			constraint != "capture_thread_verdict_thread_user_key" {
			return 0, fmt.Errorf("capture: moving thread verdicts to the thread they joined: %w",
				errors.Join(moveErr, rollbackErr))
		}
	}
	var left int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM capture_thread_verdict WHERE thread_key = $1`, from).Scan(&left); err != nil {
		return 0, fmt.Errorf("capture: counting the verdicts still on the retired thread: %w", err)
	}
	return left, nil
}
