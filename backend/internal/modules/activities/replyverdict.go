// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Whether an inbound reply was positive, negative or neither.
//
// An SDR's headline number is positive-reply rate, and until this column
// nothing recorded whether a reply was positive. The waiting queue knows
// somebody answered and owed_verdict knows whether a message asks for
// something; neither says whether the answer was a yes.
//
// THE VERDICT RIDES THE CLASSIFY PASS. capture_classify already reads this
// backlog and already spends one model call per ten messages, so this is
// another field in that call's answer rather than a second pass over the same
// mail. The write is separate from SetCaptureLabel because the two answer
// different questions about different populations — a label routes attention on
// mail in both directions, a reply verdict judges inbound mail only — but they
// arrive together and cost one call between them.
//
// THE WRITE IS AUDITED AND EVENTED, where capture_label beside it is neither.
// The label's exemption is a stated hard-floor rule about routing attention; it
// covers that column. This is a model-derived claim about what a customer
// meant, and it feeds a number a rep is measured on, so "which records did the
// classifier touch" has to stay answerable from audit_log like every other
// derived write in this tree.
//
// UNJUDGED IS A REAL ANSWER, spelled NULL. A rate counts an unjudged reply in
// neither its numerator nor its denominator, so a week of mostly-unjudged mail
// reports a small denominator rather than a confident wrong number. There is
// deliberately no `unknown` enum value: a fourth value would let a row claim the
// classifier concluded "unknown" when what happened is that it never reached the
// confidence floor.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var errNoActorForCorrection = errors.New(
	"activities: a reply-verdict correction needs an authenticated actor to record it against")

// The closed set of reply verdicts. Three, and the absent fourth is the point:
// "unknown" is the absence of a verdict and is spelled NULL, the same way
// owed_verdict spells unjudged.
const (
	// ReplyVerdictPositive is a reply that moves the conversation forward —
	// interest, a question worth answering, an ask to meet.
	ReplyVerdictPositive = "positive"
	// ReplyVerdictNegative is a reply that closes it: not interested, wrong
	// person with no referral, stop writing.
	ReplyVerdictNegative = "negative"
	// ReplyVerdictNeutral is a reply that is neither — an out-of-office, an
	// acknowledgement, a redirect to somebody else without a judgement.
	ReplyVerdictNeutral = "neutral"
)

// UnjudgedReply is one reply-backlog row as the classify prompt consumes it.
type UnjudgedReply struct {
	ID      ids.UUID
	Subject string
	Body    string // pre-truncated by the caller's body limit
}

// ReplyBacklogPredicate is what makes an activity eligible for a reply verdict.
//
// It is the classify backlog's own exclusions plus direction: the audience and
// hold clauses are here for the reason they are there — a message the worklist
// may not open is not one to spend a model call on, and a row under a statutory
// hold is out of reach of every ordinary read path.
//
// INBOUND ONLY, which is the clause that is not shared. The classify backlog
// carries both directions because a label routes attention either way; judging
// our own outbound mail as though it were a customer's answer would put the
// workspace's own words into a rate about what customers said.
//
// The unqualified `activity` reference matches ClassifyBacklogPredicate's
// posture beside it: the caller queries the table under its own name.
const ReplyBacklogPredicate = `reply_verdict IS NULL
	  AND direction = 'inbound'
	  AND captured_by LIKE 'connector:%' AND kind IN ('email', 'message')
	  AND archived_at IS NULL
	  AND audience = 'workspace'
	  AND restricted_at IS NULL`

// UnjudgedReplies reads the oldest inbound correspondence carrying no reply
// verdict — the partial-index backlog (idx_activity_unjudged_reply).
func (s *Store) UnjudgedReplies(ctx context.Context, limit, bodyLimit int) ([]UnjudgedReply, error) {
	// Reading the backlog reads customer correspondence, so it takes the same
	// object gate every other read of this table takes. No row-scope clause
	// beside it: the pass runs as the system principal over the whole
	// workspace, and the predicate's own audience and hold clauses are what
	// bound which messages it may be given.
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []UnjudgedReply
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, coalesce(subject, ''), coalesce(left(body, $1), '')
			FROM activity
			WHERE `+ReplyBacklogPredicate+`
			ORDER BY occurred_at
			LIMIT $2`, bodyLimit, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m UnjudgedReply
			if err := rows.Scan(&m.ID, &m.Subject, &m.Body); err != nil {
				return err
			}
			out = append(out, m)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("activities: reading the reply backlog: %w", err)
	}
	return out, nil
}

// SetReplyVerdict writes one verdict and its first history row, reporting
// whether it applied.
//
// The reply_verdict IS NULL predicate is the CAS: a concurrent pass that judged
// the row first wins and this write reports applied=false — the earlier verdict
// stands, never an overwrite.
//
// All THREE exclusions are re-tested at WRITE time, not only in the backlog that
// selected this row, for the reasons SetCaptureLabel states beside it: the
// classifier reads a batch, spends a model call, and writes the answers back,
// and the row can change inside that window. The archive takes the message off
// the timeline; a narrowing limits who may read it; a statutory hold puts it out
// of reach of every ordinary path. A verdict landing after any of the three
// would be a derived claim on a message the writer may no longer touch, and
// nothing revisits it, because each of those transitions has already run.
func (s *Store) SetReplyVerdict(ctx context.Context, id ids.UUID, verdict, classifier string) (applied bool, err error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return false, err
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE activity
			   SET reply_verdict = $2, reply_verdict_at = now(), reply_verdict_by = $3
			 WHERE id = $1 AND reply_verdict IS NULL AND direction = 'inbound'
			   AND archived_at IS NULL AND audience = 'workspace' AND restricted_at IS NULL`,
			id, verdict, classifier)
		if err != nil {
			return fmt.Errorf("activities: setting reply verdict: %w", err)
		}
		applied = tag.RowsAffected() > 0
		if !applied {
			return nil
		}
		if err := appendReplyVerdictHistory(ctx, tx, id, &verdict, classifier, false); err != nil {
			return err
		}
		// Audited in the SAME transaction as the column write, so a verdict that
		// landed is one audit_log can account for — the question this tree
		// answers for every derived write, asked of the pass that reads customer
		// correspondence and puts a number on what it read.
		//
		// No outbox row beside it, matching owed_verdict's reasoning: the write
		// shape asks for an event where one has a consumer outside the
		// transaction, and this has none. The verdict is read from the column by
		// whatever computes the rate next.
		if _, err := storekit.AuditEvent(ctx, tx, "update", "activity", id,
			map[string]any{"reply_verdict": verdict, "reply_verdict_by": classifier}); err != nil {
			return err
		}
		return nil
	})
	return applied, err
}

// CorrectReplyVerdict records a human disagreeing with the standing verdict.
//
// APPEND, NOT EDIT. The activity keeps the current answer so every read stays
// one row, and the history table records how it got there. A rate that moved
// because a rep re-judged twenty replies is a different fact from one that moved
// because customers changed their minds, and an overwriting column cannot tell
// those apart afterwards.
//
// A nil verdict is a correction back to unjudged, which a human may legitimately
// make: "nobody can tell from this message" is an answer, and it returns the row
// to counting in neither half of the rate.
func (s *Store) CorrectReplyVerdict(ctx context.Context, id ids.UUID, verdict *string) error {
	// A correction is a human act and is recorded under the human who made it.
	// An unauthenticated caller has no name to record, and writing one anyway
	// would put an anonymous entry in an append-only history whose whole purpose
	// is saying who decided what.
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return err
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return errNoActorForCorrection
	}
	who := actor.ID
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		// A correction may not reach a row an ordinary read cannot: the same
		// three exclusions the classifier's own write re-tests.
		tag, err := tx.Exec(ctx, `
			UPDATE activity
			   SET reply_verdict = $2,
			       reply_verdict_at = CASE WHEN $2::text IS NULL THEN NULL ELSE now() END,
			       reply_verdict_by = CASE WHEN $2::text IS NULL THEN NULL ELSE $3::text END
			 WHERE id = $1 AND direction = 'inbound'
			   AND archived_at IS NULL AND audience = 'workspace' AND restricted_at IS NULL`,
			id, verdict, who)
		if err != nil {
			return fmt.Errorf("activities: correcting reply verdict: %w", err)
		}
		// ErrNotFound rather than a distinct "cannot be judged" error: an
		// archived, narrowed or held row must not be distinguishable from one
		// that does not exist, or the refusal itself discloses that a message
		// the caller may not read is there.
		if tag.RowsAffected() == 0 {
			return apperrors.ErrNotFound
		}
		if err := appendReplyVerdictHistory(ctx, tx, id, verdict, who, true); err != nil {
			return err
		}
		if _, err := storekit.AuditEvent(ctx, tx, "update", "activity", id,
			map[string]any{"reply_verdict": verdict, "corrected": true}); err != nil {
			return err
		}
		return nil
	})
}

// appendReplyVerdictHistory writes one row of the append-only record. Both
// writers go through it, so the classifier's own judgement and a human
// correction cannot come to be recorded two different ways.
func appendReplyVerdictHistory(
	ctx context.Context, tx pgx.Tx, id ids.UUID, verdict *string, decidedBy string, isHuman bool,
) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_reply_verdict_history (activity_id, verdict, decided_by, is_human)
		VALUES ($1, $2, $3, $4)`, id, verdict, decidedBy, isHuman); err != nil {
		return fmt.Errorf("activities: recording the reply verdict: %w", err)
	}
	return nil
}
