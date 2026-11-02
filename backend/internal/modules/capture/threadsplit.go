// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ThreadVerdictCopySeatTx retains the source answer and never weakens an
// existing destination hold. A newborn partition can have no representative yet.
func ThreadVerdictCopySeatTx(ctx context.Context, tx pgx.Tx, from, to string, seat ids.UUID) error {
	if from == "" || to == "" || from == to {
		return nil
	}
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM capture_thread_verdict
 WHERE thread_key=@from AND user_id=@seat FOR UPDATE`, pgx.NamedArgs{"from": from, threadArgSeat: seat}).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("capture: reading a conversation's inherited verdict: %w", err)
	}
	if verdictHoldRank(status) <= 1 {
		return nil
	}
	weaker := weakerThreadVerdicts(status)
	if _, err := tx.Exec(ctx, `INSERT INTO capture_thread_verdict
 (thread_key,user_id,status,kind,confidence,seen_addresses,resolved_at,next_attempt_at,first_activity_id)
 SELECT @to,user_id,status,kind,confidence,seen_addresses,resolved_at,
 CASE WHEN status='pending' THEN now() ELSE NULL END,NULL
 FROM capture_thread_verdict WHERE thread_key=@from AND user_id=@seat
 ON CONFLICT (thread_key,user_id) DO UPDATE SET
 status=EXCLUDED.status,kind=EXCLUDED.kind,confidence=EXCLUDED.confidence,
 seen_addresses=EXCLUDED.seen_addresses,resolved_at=EXCLUDED.resolved_at,
 next_attempt_at=EXCLUDED.next_attempt_at,claimed_by=NULL,claimed_until=NULL,updated_at=now()
 WHERE capture_thread_verdict.status=ANY(@weaker::text[])`, pgx.NamedArgs{"from": from, "to": to, threadArgSeat: seat, "weaker": weaker}); err != nil {
		return fmt.Errorf("capture: inheriting one seat's conversation verdict: %w", err)
	}
	return ThreadVerdictRepairSeatTx(ctx, tx, to, seat)
}

func weakerThreadVerdicts(status string) []string {
	statuses := []string{VerdictPending, VerdictCleared, VerdictHeld, VerdictUnsure, VerdictSharedByOwner, VerdictHeldByOwner}
	weaker := make([]string, 0, len(statuses))
	for _, candidate := range statuses {
		if verdictHoldRank(candidate) < verdictHoldRank(status) {
			weaker = append(weaker, candidate)
		}
	}
	return weaker
}

// ThreadVerdictRepairSeatTx replaces a representative that moved to another
// partition; a verdict without any surviving message keeps its hold but no text.
func ThreadVerdictRepairSeatTx(ctx context.Context, tx pgx.Tx, key string, seat ids.UUID) error {
	if _, err := tx.Exec(ctx, `UPDATE capture_thread_verdict v SET first_activity_id=(
 SELECT a.id FROM activity a
 WHERE a.thread_key=@key AND a.archived_at IS NULL
 AND EXISTS(SELECT 1 FROM capture_import ci WHERE ci.activity_id=a.id AND ci.user_id=@seat)
 ORDER BY a.occurred_at DESC,a.id DESC LIMIT 1),updated_at=now()
 WHERE v.thread_key=@key AND v.user_id=@seat
 AND (v.first_activity_id IS NULL OR NOT EXISTS(
 SELECT 1 FROM activity a WHERE a.id=v.first_activity_id AND a.thread_key=@key
 AND a.archived_at IS NULL AND EXISTS(
 SELECT 1 FROM capture_import ci WHERE ci.activity_id=a.id AND ci.user_id=@seat)))`, pgx.NamedArgs{threadArgKey: key, threadArgSeat: seat}); err != nil {
		return fmt.Errorf("capture: repairing one seat's conversation representative: %w", err)
	}
	return nil
}
