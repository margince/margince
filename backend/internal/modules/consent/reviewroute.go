// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What is left of asking somebody else to decide a refused send.
//
// A rep no longer hands a refusal to a colleague. A rep overrules a machine
// refusal with a reason of their own (POST /contacts/{id}/consent/allow), and an
// admin may direct a send from the review. The router below only closes cards
// raised before that change: withdrawing one, or handing its review back when it
// is declined or runs out.

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// FieldReviewID is the review's name on the wire, in an audit payload and in a
// refusal's field. One spelling, because a typo in any of the three would name
// a field the caller did not send.
// FieldReviewID is exported because compose names the same field when it builds
// the card's identity, and two spellings of one identity would stage two cards
// for one question.
const FieldReviewID = "review_id"

// ReviewRouter closes a card an earlier version raised for a review.
//
// It runs on the CALLER'S transaction, so the review moving to awaiting and the
// card appearing are one fact. A card with no review behind it is work nobody
// can complete; a review claiming to be routed with no card is a rep waiting
// for an answer that was never asked for.
type ReviewRouter interface {
	// WithdrawCardTx retracts a card whose question has been answered another
	// way.
	//
	// A rep can route their refusal and then be handed the authority, or a
	// colleague can direct the same message from the review itself. The card is
	// then asking about a message that has already gone — and approving it
	// would put a second decision on the record for one send, by somebody who
	// was shown a question that no longer exists.
	//
	// On the caller's transaction, so the review closing and the card
	// retracting are one fact.
	WithdrawCardTx(ctx context.Context, tx pgx.Tx, approvalID ids.UUID, reason string) error
}

// WithReviewRouter wires the approvals-side seam. Compose binds it.
func (s *Store) WithReviewRouter(router ReviewRouter) *Store {
	s.reviewRouter = router
	return s
}

// RequestDecision refuses: asking a colleague to decide a refused send is not
// offered any more. The refusal names the moves that are.
func (s *Store) RequestDecision(ctx context.Context, _ ids.UUID, _ string) (ids.UUID, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return ids.UUID{}, err
	}
	return ids.UUID{}, &ReviewNotRoutableError{Why: askingRetired}
}

// askingRetired is what a caller of the retired ask is told instead.
const askingRetired = "asking a colleague to decide a refused send is no longer offered; " +
	"overrule the refusal with your reason (POST /contacts/{id}/consent/allow), " +
	"or ask an admin to send it from the review"

// ReviewNotRoutableError refuses a route that would put an unanswerable
// question in front of somebody.
type ReviewNotRoutableError struct {
	Why string
}

func (e *ReviewNotRoutableError) Error() string { return e.Why }

// FieldFault carries the refusal to every surface rather than the HTTP one
// alone, and names the review because that is what the caller would look at.
func (e *ReviewNotRoutableError) FieldFault() (field, code, message string) {
	return FieldReviewID, "review_not_routable", e.Why
}

// ReturnToAskerTx puts a routed review back in front of the colleague who raised
// it, because nobody is going to decide it.
//
// A decider said no, or the card ran out its window. Either way the review must
// not stay reading awaiting_decision: the rep would be waiting on an answer
// that is never coming, and nothing on their screen would say the asking had
// ended.
//
// IT RETURNS TO NEEDS_CONTEXT, not to resolved. The message is still held and
// still refused — what has ended is the asking, not the work. The rep can route
// it again to somebody else, add the evidence the engine wanted, or give up and
// cancel it, which are exactly the moves they had before they asked.
//
// The approval_id is cleared with the move, which the routing shape CHECK
// requires of anything no longer awaiting a decision.
func ReturnToAskerTx(ctx context.Context, tx pgx.Tx, reviewID ids.UUID, why string) error {
	tag, err := tx.Exec(ctx, `
		UPDATE communication_review
		   SET state = 'needs_context', approval_id = NULL
		 WHERE id = $1 AND resolved_at IS NULL AND state = 'awaiting_decision'`, reviewID)
	if err != nil {
		return fmt.Errorf("consent: returning this review to the colleague who raised it: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Already moved on — directed from the review itself, cancelled, or
		// resolved. Not an error: the asking has ended either way, which is all
		// this was for.
		return nil
	}
	_, err = storekit.Audit(ctx, tx, "update", "communication_review", reviewID,
		map[string]any{fieldStatus: ReviewAwaitingDecision},
		map[string]any{fieldStatus: ReviewNeedsContext, "returned_because": why})
	return err
}

// AwaitingDecision lists the refused sends waiting on somebody who may decide
// them.
//
// THE DECIDER'S OWN QUEUE, and gated on the grant that makes them one. A seat
// that cannot direct a send has nothing to do with this list, and handing it to
// them would be disclosing other colleagues' refused correspondence to somebody
// with no reason to see it.
//
// EVERY WAITING REVIEW, not only the ones routed to them personally. Routing
// names no assignee: a rep asks the installation, not a colleague, and whoever
// holds the authority answers. An assignee-scoped list would leave a card
// nobody could find the moment the colleague it named went on leave.
//
// BOUNDED, because a queue read has to answer in time whatever the backlog is.
// A list at its limit is a list with more behind it, and the caller is told so
// by the count rather than by discovering it.
func (s *Store) AwaitingDecision(ctx context.Context, limit int, cursor string) ([]Review, int, string, error) {
	// A HUMAN, not merely a principal auth.RequireHuman admits. That check
	// refuses buyers and agents and lets CONNECTORS through, and a connector
	// runs with the granting human's own grants — so it would hold whatever
	// this queue is gated on and could read the installation's refused
	// correspondence wholesale. This list is a colleague's work queue.
	if err := requireAHumanAtTheKeyboard(ctx); err != nil {
		return nil, 0, "", err
	}
	// READ rather than create, for ReviewForReader's reason: seeing the queue
	// and overriding the engine are different authorities, and an installation
	// can grant the first without the second.
	if err := auth.Require(ctx, entityCommunicationException, principal.ActionRead); err != nil {
		return nil, 0, "", err
	}
	if limit <= 0 || limit > maxAwaitingDecision {
		limit = maxAwaitingDecision
	}
	// The position to resume from, zero when the caller sent none.
	//
	// An undecodable token is the caller's to fix, so it is refused rather than
	// read as the start of the queue. Serving page one again is how a client
	// loops on a token it keeps re-sending.
	after := storekit.Cursor{}
	if cursor != "" {
		decoded, err := storekit.DecodeCursor(cursor)
		if err != nil {
			return nil, 0, "", err
		}
		after = decoded
	}
	var out []Review
	var total int
	var lastOpened time.Time
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// Count and page share a statement snapshot, even when another colleague
		// routes or resolves a review while this request is reading.
		// The count is taken over the whole queue, and the page cut from it in
		// the outer query. Both in one statement.
		//
		// A window count after the keyset filter counts the remainder, so
		// `total` would fall as a caller paged. A documented number would then
		// mean something else on page two.
		//
		// id breaks the tie opened_at leaves. Two reviews opened in the same
		// microsecond are otherwise ordered by nothing, and a keyset over an
		// unstable order repeats one row and skips another.
		rows, err := tx.Query(ctx, `
			SELECT q.id, q.state, q.kind, q.delivery_intent_id, q.refusals, q.reason_code,
			       q.initiated_by, q.approval_id, q.opened_at, q.total
			  FROM (
			       SELECT id, state, kind,
			              coalesce(delivery_intent_id, '00000000-0000-0000-0000-000000000000'::uuid)
			                AS delivery_intent_id,
			              refusals, reason_code,
			              coalesce(initiated_by, '00000000-0000-0000-0000-000000000000'::uuid)
			                AS initiated_by,
			              coalesce(approval_id, '00000000-0000-0000-0000-000000000000'::uuid)
			                AS approval_id,
			              opened_at, count(*) OVER () AS total
			         FROM communication_review
			        WHERE state = 'awaiting_decision' AND resolved_at IS NULL
			       ) q
			 WHERE $2::timestamptz IS NULL OR (q.opened_at, q.id) > ($2, $3)
			 ORDER BY q.opened_at, q.id
			 LIMIT $1`, limit, nullableTime(after.CreatedAt), after.ID)
		if err != nil {
			return fmt.Errorf("consent: reading the refused sends waiting for a decision: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var r Review
			var payload []byte
			if err := rows.Scan(&r.ID, &r.State, &r.Kind, &r.IntentID, &payload,
				&r.ReasonCode, &r.InitiatedBy, &r.ApprovalID, &lastOpened, &total); err != nil {
				return fmt.Errorf("consent: reading a waiting review: %w", err)
			}
			if err := json.Unmarshal(payload, &r.Refusals); err != nil {
				return fmt.Errorf("consent: reading what a waiting review was refused for: %w", err)
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, "", err
	}
	// A token only where a page was filled. A short page is the end of the
	// queue, and a cursor there offers a page that is never answered.
	if len(out) < limit {
		return out, total, "", nil
	}
	next, err := storekit.EncodeCursor(lastOpened, out[len(out)-1].ID)
	if err != nil {
		return nil, 0, "", err
	}
	return out, total, next, nil
}

// nullableTime hands Postgres NULL for the zero instant, which is how the
// keyset predicate above reads "no cursor" without a second statement.
func nullableTime(at time.Time) *time.Time {
	if at.IsZero() {
		return nil
	}
	return &at
}

// maxAwaitingDecision bounds one page of the queue. Oldest first, so a backlog
// is worked from the end that has been waiting longest rather than from
// whichever rows the planner happened to reach.
const maxAwaitingDecision = 100

// LiveReviewsForIntents answers which review, if any, stands over each of these
// held messages. It implements activities.ReviewLookup; compose binds it.
//
// GATED ON READING THE MESSAGE, and SCOPED TO THE CALLER'S OWN REVIEWS.
//
// The grant is activity:read, the same check the scheduled-send list makes
// before it has any ids to ask about. Gating on the REVIEW's own grant would
// refuse the rep whose message it is — they may not direct a send, and this is
// their own held message they are looking at.
//
// THE SCOPE IS THE HALF THAT MATTERS, and it took a review round to add. The
// caller supplies the ids, and this is an exported store method — so an
// argument resting on what today's call sites happen to pass is an argument
// about them rather than about this function, and the next caller inherits
// none of it. Without the predicate below, anybody holding activity:read could
// pass somebody else's message id and learn that their correspondence carries
// an outstanding refusal, a fact ReviewForReader hides with a 404.
//
// ON initiated_by, which is the rep who pressed send and therefore the rep the
// held message belongs to. Consent cannot read scheduled_send to check
// ownership directly — that is activities' table — and it does not need to:
// the review records who was refused, and that is the same rep.
//
// ONE READ FOR EVERY ID, because the surface that needs this is a list.
func (s *Store) LiveReviewsForIntents(
	ctx context.Context, intents []ids.UUID,
) (map[ids.UUID]ids.UUID, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	// A principal with no seat — a system worker, a connector — initiated no
	// review, so the predicate below would match nothing. Answering empty says
	// that plainly rather than sending a zero uuid into the query.
	seat := initiatingSeat(ctx)
	out := make(map[ids.UUID]ids.UUID, len(intents))
	if len(intents) == 0 || seat.IsZero() {
		// An empty map rather than a nil one: the answer is "no review stands
		// over any of these", which every caller reads the same way.
		return out, nil
	}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT delivery_intent_id, id
			  FROM communication_review
			 WHERE delivery_intent_id = ANY($1::uuid[])
			   AND resolved_at IS NULL
			   AND initiated_by = $2`, intents, seat)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var intent, review ids.UUID
			if err := rows.Scan(&intent, &review); err != nil {
				return err
			}
			out[intent] = review
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("consent: finding the reviews standing over these messages: %w", err)
	}
	return out, nil
}
