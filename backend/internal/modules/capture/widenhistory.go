// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Re-opening the mail one seat's counterparty hold caught.
//
// The only widening path in a derivation that is otherwise tighten-only, and it
// exists because without it a hold cannot be undone: place one by mistake — the
// wrong domain, a supplier typed as a lawyer — and every message it ever caught
// stays held forever, with no product path that reaches them.
//
// It is an explicit act, not a derivation. narrowhistory.go says why the
// derivation never widens on its own: the messages were held for reasons that
// were true when they landed, and a posture change is not a review of them.
// This is that review, asked for by the person whose hold it was.
//
// SEAT granularity, not per-hold. capture_import records which seat imported a
// message and that a counterparty hold caught it; it does not record WHICH hold
// matched — heldCounterpartyTx evaluates EXISTS over the seat's whole list and
// throws the answer away. So the product operation is "re-open everything my
// hold caught", and the UI has to say that rather than name one domain.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// widenBatch matches narrowBatch, and carries the same caveat: it bounds the
// statement, not the transaction.
const widenBatch = 500

// CounterpartyHoldClearer removes the row-level counterparty hold from the
// activities a widening pass re-opened.
//
// A seam rather than a statement here, for the same reason AudienceRecomputer
// is one: the activity table belongs to the activities module, and a module
// never writes a sibling's table. compose injects
// activities.ClearCounterpartyHoldTx.
type CounterpartyHoldClearer func(ctx context.Context, tx pgx.Tx, activityIDs []ids.ActivityID) error

// widenDue is which import rows a seat may re-open, and is the whole safety
// property of this file.
//
// Three clauses, each refusing a different way to publish something:
//
//   - user_id = $1 — one seat's own imports. A hold is a seat's decision and
//     re-opening it is that seat's to make; another seat's hold on the same
//     message survives, because the derivation still takes the strictest answer
//     across every import row.
//   - posture_at_import = 'held' — only rows actually held. A 'shared' or
//     'classified' row has nothing to re-open, and claiming it would rewrite a
//     posture nobody asked about.
//   - verdict_reasons = ARRAY['counterparty'] — EXACT match on a one-element
//     list, not `'counterparty' = ANY(...)`. This is the clause the whole
//     change was built for. A message held by a counterparty hold AND anything
//     else records both reasons, and `ANY` would claim it. The four other
//     reasons it therefore refuses, each of which the ladder now records even
//     when something stricter decided the audience: the confidential marker
//     (the sender asked us not to share it), the workspace floor, an inherited
//     holding verdict (the thread was held anyway), and the mailbox's own
//     posture (a standing instruction that outlives any one hold). Exact match
//     says "this hold was the ONLY reason", which is the only case a seat
//     lifting their hold is entitled to release.
//
// A row written before verdict_reasons existed has NULL there and matches
// nothing, which is intended: it records the first rule that matched and says
// nothing about the rest, so it is not PROVABLY counterparty-only. The cost is
// that pre-migration mail cannot be re-opened in bulk; the alternative is
// guessing, in the direction that discloses.
const widenDue = `user_id = $1
	   AND posture_at_import = 'held'
	   AND verdict_reasons = ARRAY['counterparty']`

// WidenRemainingTx counts what a further pass would still claim, so the caller
// can prove each pass made progress rather than trusting that it did.
func WidenRemainingTx(ctx context.Context, tx pgx.Tx, seat ids.UUID) (int, error) {
	var n int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM capture_import WHERE `+widenDue, seat).Scan(&n); err != nil {
		return 0, fmt.Errorf("capture: counting the import rows still to re-open: %w", err)
	}
	return n, nil
}

// WidenHistoryTx re-opens one bounded batch of the mail this seat's counterparty
// holds caught, and answers how many rows it moved. The caller loops until it
// answers zero.
func WidenHistoryTx(
	ctx context.Context, tx pgx.Tx, seat ids.UUID,
	recompute AudienceRecomputer, clearHold CounterpartyHoldClearer,
) (int, error) {
	rows, err := tx.Query(ctx, `
		WITH due AS (
			SELECT id FROM capture_import
			 WHERE `+widenDue+`
			 ORDER BY id
			 LIMIT $2
			 FOR UPDATE SKIP LOCKED
		)
		UPDATE capture_import i
		   SET posture_at_import = 'shared',
		       verdict_reason = NULL,
		       verdict_reasons = NULL
		  FROM due
		 WHERE i.id = due.id
		RETURNING i.activity_id`, seat, widenBatch)
	if err != nil {
		return 0, fmt.Errorf("capture: claiming import rows to re-open: %w", err)
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
		return 0, fmt.Errorf("capture: claiming import rows to re-open: %w", err)
	}
	// The row's own audience_reason has to be cleared alongside, or nothing
	// moves: rowCarriedHold reads that column as the durable record of a hold
	// no import row explains, so a row still reading 'counterparty' is re-pinned
	// by the very recompute below. The clearing lives in activities, which owns
	// the table and knows which values are its own to remove.
	if clearHold != nil {
		if err := clearHold(ctx, tx, touched); err != nil {
			return 0, err
		}
	}
	return len(touched), recomputeEach(ctx, tx, touched, recompute)
}

// ShareHistory releases every import this seat's counterparty holds caught and
// answers how many it released.
//
// RELEASED, not re-opened, and the difference is the whole of what the caller is
// told: this writes the seat's own import rows, and the activity's audience is
// still the strictest answer across every seat that imported it. A message a
// colleague also holds stays limited, so a released import is not always a
// message the workspace can now read. Counting activities that actually moved
// would mean asking the recompute what it did, which it does not report — and
// the honest number is the one this operation is responsible for.
//
// The gate is the same as every other hold operation's: a human seat, acting on
// their own imports. There is no id and no admin arm — whose mail a person keeps
// private is itself private, so an operation that reached another seat's holds
// would disclose that they have any.
//
// Audited, and the image carries the count rather than any address: what
// happened is that one seat re-opened N of their own messages, and naming the
// correspondents in a governance row would put the very list this feature keeps
// private into a log an admin reads.
func (s *CounterpartyHoldStore) ShareHistory(
	ctx context.Context, recompute AudienceRecomputer, clearHold CounterpartyHoldClearer,
) (int, error) {
	actor, err := seatItself(ctx)
	if err != nil {
		return 0, err
	}
	seat := actor.UserID
	var reopened int
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		reopened, err = widenAll(ctx, tx, seat, recompute, clearHold)
		if err != nil {
			return err
		}
		if reopened == 0 {
			// Nothing moved, so nothing happened: an audit row saying a seat
			// re-opened zero messages records a decision the product did not
			// make, and a governance reader counting them would over-count.
			return nil
		}
		// AuditEvent, not Audit: there is no before-image to record. The seat
		// did not change a setting from one value to another — they asked for
		// a batch of their own messages to be re-opened, and what happened is
		// the count.
		_, err := storekit.AuditEvent(ctx, tx, "update", captureSettingsObject, seat,
			map[string]any{"reopened_by_counterparty_hold": reopened})
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("capture: re-opening the mail a counterparty hold caught: %w", err)
	}
	return reopened, nil
}

// widenAll runs bounded passes until nothing is left to claim.
//
// Progress is checked rather than assumed, the same shape narrowHistoryTo uses
// and for the same reason: the loop ends only while the batch predicate excludes
// what it writes, and a predicate that stopped doing so would spin here forever
// holding this transaction's locks. Every pass must leave FEWER rows than the
// one before, so a repeat fails on the second pass rather than after a ceiling
// nobody would wait out.
func widenAll(
	ctx context.Context, tx pgx.Tx, seat ids.UUID,
	recompute AudienceRecomputer, clearHold CounterpartyHoldClearer,
) (int, error) {
	total, remaining := 0, -1
	for {
		moved, err := WidenHistoryTx(ctx, tx, seat, recompute, clearHold)
		if err != nil {
			return 0, err
		}
		if moved == 0 {
			return total, nil
		}
		total += moved
		left, err := WidenRemainingTx(ctx, tx, seat)
		if err != nil {
			return 0, err
		}
		if remaining >= 0 && left >= remaining {
			return 0, fmt.Errorf(
				"capture: re-opening a hold's history made no progress: %d rows still due after a pass that moved %d",
				left, moved)
		}
		remaining = left
	}
}
