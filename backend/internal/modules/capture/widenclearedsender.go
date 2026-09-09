// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Re-opening the mail a mailbox held while it waited for a verdict on the sender.
//
// A `classified` mailbox asks one question of every message: who is this from?
// Until something answers, the message is held on the posture. When the verdict
// answers "a real person", the question the hold was waiting for is settled, and
// the mail it held has no remaining reason to stay limited.
//
// widenhistory.go re-opens what a SEAT's counterparty hold caught; this re-opens
// what a POSTURE held about one SENDER. The shapes are deliberately the same —
// exact-array predicate, bounded batches, progress-proving loop — because both
// are widening in a derivation that is otherwise tighten-only, and both must
// prove they release only what they are entitled to.
//
// No hold-clearing seam here, unlike widenhistory.go: 'posture' is not one of the
// reasons activities carries on the row itself (rowCarriedHold reads only
// no_record, no_counterparty, workspace_floor, counterparty and
// explicitly_confidential), so rewriting the import rows is enough for the
// recompute to reach a new answer.

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// clearedSenderBatch bounds the statement, matching widenBatch.
const clearedSenderBatch = 500

// clearedSenderWidenDue is which import rows a settled real/person verdict may
// re-open, and it is the whole safety property of this file.
//
// The ledger clause is FIRST and is not optional. A caller cannot establish
// real/person by having just written it: createCounterparty answers nil after
// correcting its own resolution to `suppressed`, and a review acceptance can
// update no row at all. So the entitlement is re-read here, inside the same
// statement that claims the rows, and every caller inherits it.
//
//   - a settled ledger row, status 'real' AND kind 'person' — never status
//     alone. advisor, role_mailbox and organization_sender are all 'real' and
//     none of them is a person whose mail a posture should stop holding.
//   - posture_at_import = 'classified' — never 'held'. A held mailbox promises
//     to hold whatever any classifier concludes, so a verdict is not an answer
//     to it; 'classified' is the posture that was waiting for exactly this.
//   - verdict_status IS NULL — a thread confidentiality verdict that has spoken
//     is not ours to move. That verdict is written onto the import row without
//     touching verdict_reasons, so the exact-array clause below does NOT imply
//     it stayed silent.
//   - and no HOLDING verdict on the thread itself, asked of that ledger rather
//     than of the import row. The row's own status is not proof the thread has
//     none: stamping a settled verdict onto a thread's siblings is bounded per
//     transaction, so a long thread keeps unstamped rows reading NULL for as
//     long as the repair takes. Those rows are exactly the mail a seat's
//     confidentiality verdict just held, and the pass that would eventually
//     stamp them arrives after this one would have published them.
//   - verdict_reasons = ARRAY['posture'] — EXACT match, the same argument
//     widenDue makes: a message held by the posture AND anything else records
//     both, and releasing it would discard the other reason. A row written
//     before verdict_reasons existed has NULL and matches nothing, so
//     pre-migration mail is not re-opened in bulk rather than guessed at.
//   - the activity's own counterparty_email — the sender the ledger row is
//     about, in the one normalization both tables share.
//
// What it therefore leaves held, deliberately: a message whose stored
// counterparty is a colleague or the owner's own alias, because the ladder
// substituted an external recipient when it asked the verdict question. Those
// rows are not provably about this sender, and matching any cleared participant
// instead would release mail on the strength of somebody else's clearance.
const clearedSenderWidenDue = `EXISTS (
			SELECT 1 FROM capture_pending_counterparty p
			 WHERE p.email = $1 AND p.status = 'real' AND p.kind = 'person')
	   AND i.posture_at_import = 'classified'
	   AND i.verdict_status IS NULL
	   AND NOT EXISTS (
			SELECT 1 FROM capture_thread_verdict v
			 WHERE v.thread_key = a.thread_key AND v.user_id = i.user_id
			   AND v.status IN ('held', 'unsure', 'held_by_owner', 'pending'))
	   AND i.verdict_reasons = ARRAY['posture']
	   AND a.counterparty_email = $1
	   AND a.restricted_at IS NULL`

// ClearedSenderRemainingTx counts what a further pass would still claim, so a
// caller can prove each pass made progress rather than trusting that it did.
func ClearedSenderRemainingTx(ctx context.Context, tx pgx.Tx, email string) (int, error) {
	var n int
	err := tx.QueryRow(ctx, `
		SELECT count(*)
		  FROM capture_import i
		  JOIN activity a ON a.id = i.activity_id
		 WHERE `+clearedSenderWidenDue, normalizeEmail(email)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("capture: counting the mail a cleared sender's verdict re-opens: %w", err)
	}
	return n, nil
}

// WidenClearedSenderTx re-opens one bounded batch of the mail a posture held
// about this sender, and answers how many rows it moved. The caller loops until
// it answers zero, or until its own budget runs out.
//
// limit is the caller's remaining budget rather than a constant, because the
// budget has to interrupt the drain itself: a single sender with thousands of
// held messages would otherwise hold one transaction open for all of them, and a
// cap checked only between senders would never fire.
func WidenClearedSenderTx(
	ctx context.Context, tx pgx.Tx, email string, limit int, recompute AudienceRecomputer,
) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	if limit > clearedSenderBatch {
		limit = clearedSenderBatch
	}
	rows, err := tx.Query(ctx, `
		WITH due AS (
			SELECT i.id
			  FROM capture_import i
			  JOIN activity a ON a.id = i.activity_id
			 WHERE `+clearedSenderWidenDue+`
			 ORDER BY i.id
			 LIMIT $2
			 FOR UPDATE OF i SKIP LOCKED
		)
		UPDATE capture_import i
		   SET posture_at_import = 'shared',
		       verdict_reason = NULL,
		       verdict_reasons = NULL
		  FROM due
		 WHERE i.id = due.id
		RETURNING i.activity_id`, normalizeEmail(email), limit)
	if err != nil {
		return 0, fmt.Errorf("capture: claiming the import rows a cleared sender re-opens: %w", err)
	}
	var touched []ids.ActivityID
	for rows.Next() {
		var id ids.ActivityID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, fmt.Errorf("capture: reading a claimed import row: %w", err)
		}
		touched = append(touched, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("capture: claiming the import rows a cleared sender re-opens: %w", err)
	}
	// Sorted before recomputing, because the recompute takes FOR UPDATE on each
	// activity in turn. Two passes claiming different seats' import rows for an
	// overlapping set of activities would otherwise take those row locks in
	// whatever order each UPDATE returned them, which is a deadlock the import
	// rows' own SKIP LOCKED cannot prevent. One agreed order removes it.
	sortActivityIDs(touched)
	return len(touched), recomputeEach(ctx, tx, touched, recompute)
}

// sortActivityIDs puts a claimed batch into one agreed order before anything
// locks the rows it names.
func sortActivityIDs(list []ids.ActivityID) {
	slices.SortFunc(list, func(a, b ids.ActivityID) int {
		return bytes.Compare(a.UUID[:], b.UUID[:])
	})
}

// ClearedSendersDueTx names the settled senders that still have mail a posture
// is holding, so a reconciling pass can drain what the live path missed.
//
// Senders rather than rows: the widen claims per sender, and asking for the work
// this way lets each one take its own transaction.
//
// It asks the SAME predicate the claim asks, with the sender's own address
// substituted for the parameter, because a selector that admits what the writer
// refuses never drains: the pass would find the sender every tick, move nothing,
// and its progress check would fail a run that had no work to do. Spelling the
// clauses again here is what would let the two drift apart.
func ClearedSendersDueTx(ctx context.Context, tx pgx.Tx, limit int) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT a.counterparty_email
		  FROM capture_import i
		  JOIN activity a ON a.id = i.activity_id
		 WHERE `+strings.ReplaceAll(clearedSenderWidenDue, "$1", "a.counterparty_email")+`
		 ORDER BY a.counterparty_email
		 LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("capture: reading the senders whose cleared mail is still held: %w", err)
	}
	defer rows.Close()
	var senders []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("capture: reading a sender whose cleared mail is still held: %w", err)
		}
		senders = append(senders, email)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("capture: reading the senders whose cleared mail is still held: %w", err)
	}
	return senders, nil
}

// WidenClearedSenderAll drains this sender within the caller's budget, proving
// progress on every pass.
//
// The progress check is the same one widenAll makes, and for the same reason:
// the loop terminates only while the predicate excludes what the statement
// writes, and a predicate that stopped doing so would spin here holding this
// transaction's locks. Every pass must leave strictly fewer rows due than the
// one before.
func WidenClearedSenderAll(
	ctx context.Context, tx pgx.Tx, email string, budget int, recompute AudienceRecomputer,
) (int, error) {
	total, remaining := 0, -1
	for total < budget {
		moved, err := WidenClearedSenderTx(ctx, tx, email, budget-total, recompute)
		if err != nil {
			return 0, err
		}
		if moved == 0 {
			return total, nil
		}
		total += moved
		left, err := ClearedSenderRemainingTx(ctx, tx, email)
		if err != nil {
			return 0, err
		}
		if remaining >= 0 && left >= remaining {
			return 0, fmt.Errorf(
				"capture: re-opening a cleared sender's mail made no progress: %d rows still due after a pass that moved %d",
				left, moved)
		}
		remaining = left
	}
	return total, nil
}

// senderClearedPersonTx answers whether this address already has a settled
// verdict saying it is a real person.
//
// Deliberately narrower than the disposition lookup the sink makes when it
// decides whether to CREATE a contact: that one also counts an existing person
// record as evidence, and a person record can exist for reasons that never
// judged the sender. Opening a mailbox's held mail is a disclosure, so it asks
// only for the verdict itself.
func senderClearedPersonTx(ctx context.Context, tx pgx.Tx, email string) (bool, error) {
	folded := normalizeEmail(email)
	if folded == "" {
		return false, nil
	}
	var cleared bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM capture_pending_counterparty
			 WHERE email = $1 AND status = 'real' AND kind = 'person')`,
		folded).Scan(&cleared)
	if err != nil {
		return false, fmt.Errorf("capture: reading whether this sender is already judged a person: %w", err)
	}
	return cleared, nil
}
