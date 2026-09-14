// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Asking somebody else to decide a refused send.
//
// A rep presses send, the engine refuses, and a review records what was refused
// and for whom. Directing that message out anyway takes an authority most seats
// do not hold — so the rep's own next move is usually not "override it", it is
// "ask someone who can".
//
// This is that ask. It puts the review in front of a decision-maker as an
// approval card, and the card's effect is the directed send itself: whoever
// approves it is the human whose name goes on the instruction.
//
// THE ROUTE IS NOT THE DECISION. Asking costs nothing and grants nothing — a
// rep who cannot direct a send still cannot, and the card they raised is
// answered by somebody who can or by nobody at all. What routing does is make
// the question findable, which is the whole difference between a refusal a rep
// can act on and one they can only read.
//
// WHY THE SEAM. Approvals is a sibling module, so consent may not reach into
// it: the router is declared here and compose binds it (ADR-0054). What crosses
// is one call — put this review in front of a decider — and what comes back is
// the card's id.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
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

// ReviewRouter stages one review as a decision somebody else can take.
//
// It runs on the CALLER'S transaction, so the review moving to awaiting and the
// card appearing are one fact. A card with no review behind it is work nobody
// can complete; a review claiming to be routed with no card is a rep waiting
// for an answer that was never asked for.
type ReviewRouter interface {
	RouteForDecisionTx(ctx context.Context, tx pgx.Tx, in ReviewRouteRequest) (ids.UUID, error)

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

// ReviewRouteRequest is what a decider needs to see before they answer.
type ReviewRouteRequest struct {
	ReviewID ids.UUID
	// IntentID names the held message. The card's effect resumes it, and a
	// route with nothing to resume is a decision about a message that no longer
	// exists.
	IntentID ids.UUID
	// ReasonCode is the strongest reason across the recipients, so a queue can
	// show what this is about without opening the snapshot.
	ReasonCode string
	// Recipients is how many contacts the refusal names, which is the other half
	// of what makes a card readable at a glance. The addresses themselves stay
	// on the review.
	Recipients int
	// Note is what the colleague asking wants the decider to know. Optional: a
	// refusal is often self-explanatory.
	Note string
}

// WithReviewRouter wires the approvals-side seam. Compose binds it.
func (s *Store) WithReviewRouter(router ReviewRouter) *Store {
	s.reviewRouter = router
	return s
}

// RequestDecision puts a refused send in front of somebody who may direct it.
//
// GATED ON READING THE REVIEW, not on directing a send. That is the point: the
// caller is asking precisely because they cannot direct it themselves. What
// they must be is the colleague whose send was refused — a seat that could route
// anybody's review would be raising cards about other colleagues' correspondence.
func (s *Store) RequestDecision(ctx context.Context, reviewID ids.UUID, note string) (ids.UUID, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return ids.UUID{}, err
	}
	if s.reviewRouter == nil {
		// A composition with no approvals engine wired cannot route, and
		// saying so is better than staging nothing and reporting success.
		return ids.UUID{}, errors.New(
			"consent: this installation has no approvals surface to route a decision to")
	}
	// BOUNDED WHERE IT IS WRITTEN, not only at the door.
	//
	// The note becomes the instruction's explanation when somebody approves,
	// and that column refuses more than maxExplanationRunes. A longer note
	// would stage a card that cannot be approved: the decider presses approve,
	// the redemption commits, and the send fails on a length nobody can see
	// from the queue.
	//
	// Refused here rather than truncated. What the record says the approver
	// acted on has to be what they read, and silently shortening it would put
	// half a sentence on an override.
	if len([]rune(note)) > maxExplanationRunes {
		return ids.UUID{}, &ReviewNotRoutableError{
			Why: fmt.Sprintf("a note is at most %d characters; this one would not fit the record "+
				"the approver's decision is written into", maxExplanationRunes),
		}
	}
	// THE INITIATOR'S OWN READ, which is what binds this to their message. It
	// answers not-found for somebody else's review, so a caller cannot route a
	// refusal they were never shown.
	review, err := s.ReviewForInitiator(ctx, reviewID)
	if err != nil {
		return ids.UUID{}, err
	}
	if review.IntentID.IsZero() {
		return ids.UUID{}, &ReviewNotRoutableError{
			Why: "this review holds no message to send, so there is nothing for a decision to act on",
		}
	}
	// ALREADY ROUTED IS NOT AN ERROR, it is the answer.
	//
	// A rep who presses the button again — or whose first request timed out on
	// the wire — is asking the same question, and the honest reply is the card
	// already carrying it. Refusing would tell them their ask failed when it
	// did not, and leave them with no way to find the card they raised.
	if review.State == ReviewAwaitingDecision {
		return routedCardFor(ctx, s.db, reviewID)
	}
	if review.State != ReviewNeedsContext && review.State != ReviewNeedsRepair {
		// Resolved, superseded or cancelled. There is nothing left to decide,
		// and putting it in front of somebody would ask them about a closed
		// matter.
		return ids.UUID{}, &ReviewNotRoutableError{
			Why: "this review is no longer waiting for a decision",
		}
	}
	var approvalID ids.UUID
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		approvalID, err = s.reviewRouter.RouteForDecisionTx(ctx, tx, ReviewRouteRequest{
			ReviewID:   reviewID,
			IntentID:   review.IntentID,
			ReasonCode: review.ReasonCode,
			Recipients: len(review.Refusals),
			Note:       note,
		})
		if err != nil {
			return err
		}
		return markReviewAwaitingTx(ctx, tx, reviewID, approvalID, review.State)
	})
	return approvalID, err
}

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

// markReviewAwaitingTx records that this review is now somebody else's to
// answer.
//
// The state is what stops a second route and what a surface reads to say
// "waiting for a decision" rather than "needs context" — the rep has done what
// they can, and the queue should say so.
func markReviewAwaitingTx(ctx context.Context, tx pgx.Tx, reviewID, approvalID ids.UUID, from string) error {
	tag, err := tx.Exec(ctx, `
		UPDATE communication_review
		   SET state = 'awaiting_decision', approval_id = $2
		 WHERE id = $1 AND resolved_at IS NULL
		   AND state IN ('needs_context', 'needs_repair')`, reviewID, approvalID)
	if err != nil {
		return fmt.Errorf("consent: recording that this review is waiting for a decision: %w", err)
	}
	if tag.RowsAffected() != 1 {
		// Something moved the review between the read above and here. Refusing
		// is the safe direction: a card staged against a review that has since
		// been answered is work nobody should be shown.
		return &ReviewNotRoutableError{
			Why: "this review changed while it was being routed; open it again to see where it stands",
		}
	}
	// Audited with both images: the review moves from work the rep holds to
	// work somebody else does, and the state afterwards cannot say what it
	// moved from.
	_, err = storekit.Audit(ctx, tx, "update", "communication_review", reviewID,
		// The state it ACTUALLY moved from. Writing needs_context unconditionally
		// would record a needs_repair review as having been something it was
		// not, and the before-image is the half of the audit that says what
		// changed.
		map[string]any{fieldStatus: from},
		map[string]any{fieldStatus: ReviewAwaitingDecision, "approval_id": approvalID})
	return err
}

// routedCardFor answers the card a review was already handed to.
//
// A review reading awaiting_decision names one, which the row's own shape CHECK
// requires — so an empty answer here is a row that should not exist, and saying
// so beats returning a zero id that reads as success.
func routedCardFor(ctx context.Context, db *database.DB, reviewID ids.UUID) (ids.UUID, error) {
	var approvalID ids.UUID
	err := db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT coalesce(approval_id, '00000000-0000-0000-0000-000000000000'::uuid)
			  FROM communication_review WHERE id = $1`, reviewID).Scan(&approvalID)
	})
	if err != nil {
		return ids.UUID{}, fmt.Errorf("consent: reading the card this review was handed to: %w", err)
	}
	if approvalID.IsZero() {
		return ids.UUID{}, &ReviewNotRoutableError{
			Why: "this review says it is waiting for a decision and names no card to carry it",
		}
	}
	return approvalID, nil
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
func (s *Store) AwaitingDecision(ctx context.Context, limit int) ([]Review, int, error) {
	// A HUMAN, not merely a principal auth.RequireHuman admits. That check
	// refuses buyers and agents and lets CONNECTORS through, and a connector
	// runs with the granting human's own grants — so it would hold whatever
	// this queue is gated on and could read the installation's refused
	// correspondence wholesale. This list is a colleague's work queue.
	if err := requireAHumanAtTheKeyboard(ctx); err != nil {
		return nil, 0, err
	}
	// READ rather than create, for ReviewForReader's reason: seeing the queue
	// and overriding the engine are different authorities, and an installation
	// can grant the first without the second.
	if err := auth.Require(ctx, entityCommunicationException, principal.ActionRead); err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > maxAwaitingDecision {
		limit = maxAwaitingDecision
	}
	var out []Review
	var total int
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// Count and page share a statement snapshot, even when another colleague
		// routes or resolves a review while this request is reading.
		rows, err := tx.Query(ctx, `
			SELECT id, state, kind,
			       coalesce(delivery_intent_id, '00000000-0000-0000-0000-000000000000'::uuid),
			       refusals, reason_code,
			       coalesce(initiated_by, '00000000-0000-0000-0000-000000000000'::uuid),
			       coalesce(approval_id, '00000000-0000-0000-0000-000000000000'::uuid),
			       count(*) OVER ()
			  FROM communication_review
			 WHERE state = 'awaiting_decision' AND resolved_at IS NULL
			 ORDER BY opened_at
			 LIMIT $1`, limit)
		if err != nil {
			return fmt.Errorf("consent: reading the refused sends waiting for a decision: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var r Review
			var payload []byte
			if err := rows.Scan(&r.ID, &r.State, &r.Kind, &r.IntentID, &payload,
				&r.ReasonCode, &r.InitiatedBy, &r.ApprovalID, &total); err != nil {
				return fmt.Errorf("consent: reading a waiting review: %w", err)
			}
			if err := json.Unmarshal(payload, &r.Refusals); err != nil {
				return fmt.Errorf("consent: reading what a waiting review was refused for: %w", err)
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, total, err
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
