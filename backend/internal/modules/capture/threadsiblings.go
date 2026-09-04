// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Applying one thread verdict to the messages of that thread this seat had
// ALREADY imported when the answer came back.
//
// A verdict is about a conversation, but the classifier reads one message of
// it. A message arriving after the answer inherits it (verdictinherit.go); a
// message that arrived BEFORE it did not, and stayed held at whatever its own
// import posture asked for — permanently, because the thread's question is
// answered and the unique ledger row stops a second one being opened.
//
// The admission rule is the one inheritance already applies: a sibling takes an
// opening verdict only if its own counterparty is an address the verdict
// actually read. Import ORDER is the accident being corrected here; who wrote
// the message is not.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ThreadOutcome is what applying one verdict to a thread changed.
type ThreadOutcome struct {
	// Stamped are the activities whose import row took the verdict. The caller
	// recomputes each one's audience: capture may not reach the activities
	// module, so the recompute is compose's to run.
	Stamped []ids.UUID
	// Reopened names the sibling the ledger now points at, or Nil.
	//
	// Set only for an OPENING verdict that found a message from a sender it
	// never read. That message is not published on an answer about somebody
	// else's mail; the thread goes back to pending pointing at it, and the
	// classifier reads it next.
	Reopened ids.UUID
}

// RecordOutcomeOnThreadTx stamps this seat's verdict onto every message of the
// thread it had already imported and not yet decided.
//
// Scoped to the seat whose verdict it is, like the single-message stamp beside
// it: a thread reaching two mailboxes is two people's correspondence, each may
// conclude differently, and the derivation takes the strictest of their
// answers. A stamp that ignored user_id would let one seat's `ordinary`
// publish a message their colleague's mailbox is holding.
func (s *ThreadVerdictStore) RecordOutcomeOnThreadTx(
	ctx context.Context, tx pgx.Tx, threadKey string, user ids.UUID,
	status, kind string, seen []string,
) (ThreadOutcome, error) {
	if threadKey == "" || user == ids.Nil {
		return ThreadOutcome{}, nil
	}
	undecided, err := undecidedSiblingsTx(ctx, tx, threadKey, user)
	if err != nil {
		return ThreadOutcome{}, err
	}
	var out ThreadOutcome
	for _, sib := range undecided {
		// A holding verdict takes every message: "this conversation is
		// private" is true of the whole conversation whoever wrote which part.
		// An opening one takes only what the verdict actually read.
		if holdingVerdict(status) || addressWasSeen(sib.from, seen) {
			out.Stamped = append(out.Stamped, sib.id)
			continue
		}
		if out.Reopened == ids.Nil {
			out.Reopened = sib.id
		}
	}
	if len(out.Stamped) > 0 {
		if err := stampSiblingsTx(ctx, tx, user, out.Stamped, status, kind); err != nil {
			return ThreadOutcome{}, err
		}
	}
	if out.Reopened != ids.Nil {
		if err := reopenAtSiblingTx(ctx, tx, threadKey, user, out.Reopened); err != nil {
			return ThreadOutcome{}, err
		}
	}
	return out, nil
}

// sibling is one already-imported message of the thread and who wrote it.
type sibling struct {
	id   ids.UUID
	from string
}

// undecidedSiblingsTx reads the messages of this thread that this seat imported
// and nothing has decided.
//
// `pending` counts as undecided alongside NULL: a message that inherited a
// pending verdict carries the word but no answer, and the derivation reads the
// two identically.
//
// Archived and restricted rows are excluded, matching the writer below and the
// owner's own decision path. A selector that reached rows its writer refuses
// would return the same page every tick and starve everything behind it.
func undecidedSiblingsTx(
	ctx context.Context, tx pgx.Tx, threadKey string, user ids.UUID,
) ([]sibling, error) {
	rows, err := tx.Query(ctx, `
		SELECT a.id, lower(btrim(coalesce(a.counterparty_email, '')))
		  FROM activity a
		  JOIN capture_import i ON i.activity_id = a.id AND i.user_id = $2
		 WHERE a.thread_key = $1
		   AND a.archived_at IS NULL AND a.restricted_at IS NULL
		   AND (i.verdict_status IS NULL OR i.verdict_status = $3)
		 ORDER BY a.occurred_at, a.id`, threadKey, user, VerdictPending)
	if err != nil {
		return nil, fmt.Errorf("capture: reading a thread's undecided messages: %w", err)
	}
	defer rows.Close()
	var out []sibling
	for rows.Next() {
		var s sibling
		if err := rows.Scan(&s.id, &s.from); err != nil {
			return nil, fmt.Errorf("capture: reading a thread's undecided messages: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("capture: reading a thread's undecided messages: %w", err)
	}
	return out, nil
}

// stampSiblingsTx writes the verdict onto the import rows named.
//
// No audit row of its own, for the same reason RecordOutcomeTx writes none: a
// capture_import row is one seat's CONTRIBUTION to a derivation, not a domain
// record, and what a reader is owed a trail of is the message's audience. Every
// change to that goes through activities.RecomputeAudienceTx, which audits and
// emits when the row actually moves. A stamp that moves nothing — because a
// colleague's mailbox is still holding the message — has nothing to report, and
// an audit row saying otherwise would claim a disclosure that did not happen.
//
// The undecided predicate is restated in the UPDATE rather than trusted from
// the read: between the two, another pass may have decided one of these rows,
// and overwriting a decision with one taken before it is how a held message
// becomes an open one.
func stampSiblingsTx(
	ctx context.Context, tx pgx.Tx, user ids.UUID, ids_ []ids.UUID, status, kind string,
) error {
	_, err := tx.Exec(ctx, `
		UPDATE capture_import
		   SET verdict_status = $3, verdict_reason = NULLIF($4, '')
		 WHERE user_id = $1 AND activity_id = ANY($2)
		   AND (verdict_status IS NULL OR verdict_status = $5)`,
		user, ids_, status, kind, VerdictPending)
	if err != nil {
		return fmt.Errorf("capture: recording a thread verdict on its earlier messages: %w", err)
	}
	return nil
}

// reopenAtSiblingTx sends the thread back for an answer about the message the
// verdict could not speak for.
//
// Unlike the arrival-path re-open this KEEPS seen_addresses and points the
// ledger at a specific message rather than clearing the pointer. Both matter:
// the pointer is what the classifier reads, and the accumulated addresses are
// what stops the senders this verdict already cleared from re-opening the
// thread again the next time one of them writes.
//
// Due on the NEXT pass rather than immediately. A thread re-opened inside the
// pass that re-opened it is claimed again by that same pass, which turns one
// answer per cycle into a chain of them: the budget a pass was given is spent
// on one thread, and every cohort of a long conversation is judged in a single
// unbounded run. The delay is what keeps a pass's cost proportional to the
// threads it claimed.
func reopenAtSiblingTx(
	ctx context.Context, tx pgx.Tx, threadKey string, user ids.UUID, at ids.UUID,
) error {
	_, err := tx.Exec(ctx, `
		UPDATE capture_thread_verdict
		   SET status = $4, first_activity_id = $3,
		       resolved_at = NULL, next_attempt_at = now() + interval '1 minute',
		       claimed_until = NULL, claimed_by = NULL, updated_at = now()
		 WHERE thread_key = $1 AND user_id = $2
		   AND status IN ($5, $6)`,
		threadKey, user, at, VerdictPending, VerdictCleared, VerdictSharedByOwner)
	if err != nil {
		return fmt.Errorf("capture: re-opening a thread for the message its verdict did not read: %w", err)
	}
	return nil
}
