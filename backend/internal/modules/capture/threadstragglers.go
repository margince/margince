// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Threads whose question is over and whose messages never heard the answer.
//
// The verdict path applies an answer to the messages it was about as it commits
// it. Three populations that path cannot reach: a thread retired to `unsure`
// without an apply ever running, a thread judged before that pass existed, and
// a thread whose apply lost the claim race after the ledger was already
// written. Each leaves a settled ledger row above messages still carrying no
// decision, held forever under a question nobody will ask again.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SettledThread names one seat's finished question and the answer it reached.
type SettledThread struct {
	ThreadKey string
	UserID    ids.UUID
	Status    string
	Kind      string
	Seen      []string
}

// ThreadsWithUndecidedMessages lists the settled questions whose messages never
// took the answer.
//
// The undecided predicate is the one stampSiblingsTx applies, restated here
// deliberately rather than approximated: a selector that offered rows its
// writer refuses would return the same page every tick and starve everything
// behind it. Archived and restricted messages are excluded on both sides.
func (s *ThreadVerdictStore) ThreadsWithUndecidedMessages(
	ctx context.Context, limit int,
) ([]SettledThread, error) {
	var out []SettledThread
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT v.thread_key, v.user_id, v.status, coalesce(v.kind, ''), v.seen_addresses
			  FROM capture_thread_verdict v
			 WHERE v.status IN ($2, $3, $4, $5, $6)
			   AND EXISTS (
			         SELECT 1
			           FROM capture_import i
			           JOIN activity a ON a.id = i.activity_id
			          WHERE i.user_id = v.user_id
			            AND a.thread_key = v.thread_key
			            AND a.archived_at IS NULL AND a.restricted_at IS NULL
			            AND (i.verdict_status IS NULL OR i.verdict_status = $7))
			 ORDER BY v.updated_at
			 LIMIT $1`,
			limit, VerdictCleared, VerdictHeld, VerdictUnsure,
			VerdictSharedByOwner, VerdictHeldByOwner, VerdictPending)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var t SettledThread
			if err := rows.Scan(&t.ThreadKey, &t.UserID, &t.Status, &t.Kind, &t.Seen); err != nil {
				return err
			}
			out = append(out, t)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("capture: listing settled threads whose messages are undecided: %w", err)
	}
	return out, nil
}

// LockSettledThreadTx takes the ledger row and re-reads the answer under the
// lock, reporting whether it is still one to apply.
//
// The sweep has no claim of its own, so two workers can read the same row
// before either writes. Locking and re-reading is what makes the pair that
// follows — stamp, and maybe re-open — derive from the state this transaction
// will actually commit against rather than from a read taken before it.
func (s *ThreadVerdictStore) LockSettledThreadTx(
	ctx context.Context, tx pgx.Tx, threadKey string, user ids.UUID,
) (SettledThread, bool, error) {
	var t SettledThread
	err := tx.QueryRow(ctx, `
		SELECT thread_key, user_id, status, coalesce(kind, ''), seen_addresses
		  FROM capture_thread_verdict
		 WHERE thread_key = $1 AND user_id = $2
		   FOR UPDATE`, threadKey, user).Scan(&t.ThreadKey, &t.UserID, &t.Status, &t.Kind, &t.Seen)
	if err != nil {
		if err == pgx.ErrNoRows {
			return SettledThread{}, false, nil
		}
		return SettledThread{}, false, fmt.Errorf("capture: locking a settled thread: %w", err)
	}
	if t.Status == VerdictPending {
		// Somebody re-opened it between the listing and the lock. A pending
		// question is the classifier's to answer, not this pass's to apply.
		return SettledThread{}, false, nil
	}
	return t, true, nil
}
