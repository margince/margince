// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What a refused send leaves behind.
//
// A refusal used to roll its whole transaction back and answer 409
// consent_not_granted. Nothing durable survived: not what was refused, not
// which recipient it was refused for, not what would change the answer. A rep
// pressed send, read a code, and had nowhere to go.
//
// The row this file writes is the difference between an error and a piece of
// work. It records the engine's answer at the moment it was given — a snapshot,
// deliberately, because a reader asking "why was this refused on Tuesday" needs
// Tuesday's answer and not what the consent rows say now.
//
// WHAT IT DOES NOT DO YET. It does not decide anything, route anything, or let
// anybody act. Opening the review and handing back its reference is what makes
// every later step possible: resuming the held message, recording the context
// that would change the answer, directing a send under a recorded exception.
// Each of those is its own slice, and each attaches here.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// The states a review can hold. Named for what is NEEDED rather than for who is
// blocked: "needs context" is a fact about the message and stays true whoever
// is looking at it, where "waiting for Anna" stops being true when Anna leaves.
const (
	// ReviewNeedsContext is a refusal that more evidence could answer — the
	// engine found no ground for this message to stand on.
	ReviewNeedsContext = "needs_context"
	// ReviewNeedsRepair is a refusal no evidence can answer, because something
	// about the message or the address is wrong.
	ReviewNeedsRepair = "needs_repair"
	// ReviewAwaitingDecision is a refusal the rep has handed to somebody who
	// may override it. It is the rep's work no longer, and a surface should say
	// so rather than showing them a form they have already filled in.
	ReviewAwaitingDecision = "awaiting_decision"
	// ReviewResolved is work somebody finished.
	ReviewResolved = "resolved"
	// ReviewSuperseded is a review replaced by a fresher attempt at the same
	// message.
	ReviewSuperseded = "superseded"
	// ReviewCancelled is a review whose message nobody intends to send.
	ReviewCancelled = "cancelled"
)

// reviewKindSingle is the only kind this slice writes: one review, one send
// attempt. The column is constrained to it so a batch review — a campaign, a
// manifest — arrives as a deliberate widening rather than as a value somebody
// wrote by accident.
const reviewKindSingle = "single"

// RefusedRecipient is one recipient's half of a refusal, as the engine gave it.
//
// The ADDRESS is stored, which is why the privacy engine clears this column
// with the subject. A reason code without an address says a message was refused
// and not who for, which is exactly the question the rep is asking.
type RefusedRecipient struct {
	Address     string `json:"address"`
	SubjectKind string `json:"subject_kind,omitempty"`
	SubjectID   string `json:"subject_id,omitempty"`
	ReasonCode  string `json:"reason_code"`
	Category    string `json:"category,omitempty"`
}

// Review is a refused send somebody can look at.
type Review struct {
	ID          ids.UUID
	State       string
	Kind        string
	IntentID    ids.UUID
	InitiatedBy ids.UUID
	Refusals    []RefusedRecipient
	ReasonCode  string
	// ApprovalID is the card a routed review was handed to. Zero on a review
	// nobody has asked about, which is most of them.
	ApprovalID ids.UUID
}

// OpenReviewTx records one refusal, inside the transaction that refused.
//
// INSIDE, because a review committed separately from the refusal can disagree
// with it: a review written after a rollback describes a send that never
// stopped, and a refusal that rolled back its own review leaves the rep with
// the error message this exists to replace.
//
// NOT GATED, and that is the point rather than an omission. This runs on the
// send path the caller has already been admitted to — they held whatever grant
// the send door required, and the engine then refused them on consent grounds.
// A second permission check here could only refuse a caller who is already
// inside, and refusing would destroy the record of what happened to them.
// Reading a review IS gated; see ReviewForInitiator.
func OpenReviewTx(ctx context.Context, tx pgx.Tx, set commsauthz.DecisionSet, intentID ids.UUID) (Review, error) {
	refusals := refusedRecipientsOf(set)
	if len(refusals) == 0 {
		// Nothing was refused, so there is no work to record. A caller that
		// asks anyway is not wrong — the set is what it is — and answering an
		// empty review is truer than inventing one.
		return Review{}, nil
	}
	payload, err := json.Marshal(refusals)
	if err != nil {
		return Review{}, fmt.Errorf("consent: recording what this send was refused for: %w", err)
	}
	// The seat is read from the principal rather than taken as an argument, for
	// the reason captured_by is everywhere else: an initiator a caller could
	// name is an initiator a caller could get wrong.
	initiator := initiatingSeat(ctx)
	out := Review{
		State:      stateFor(refusals),
		Kind:       reviewKindSingle,
		IntentID:   intentID,
		Refusals:   refusals,
		ReasonCode: refusals[0].ReasonCode,
	}
	// ON CONFLICT on the live-intent index: a second attempt at the same held
	// message updates the standing review rather than opening a rival. Two live
	// reviews for one message would put the same work in front of somebody
	// twice, and resolving one would leave the other pointing at a message that
	// has already gone.
	err = tx.QueryRow(ctx, `
		INSERT INTO communication_review
		  (state, kind, delivery_intent_id, initiated_by, refusals, reason_code)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (delivery_intent_id)
		  WHERE delivery_intent_id IS NOT NULL AND resolved_at IS NULL
		  DO UPDATE SET refusals = EXCLUDED.refusals,
		                reason_code = EXCLUDED.reason_code,
		                state = EXCLUDED.state,
		                opened_at = now()
		RETURNING id`,
		out.State, out.Kind, zeroUUIDAsNull(intentID), zeroUUIDAsNull(initiator),
		payload, out.ReasonCode).Scan(&out.ID)
	if err != nil {
		return Review{}, fmt.Errorf("consent: opening the review this refusal owes: %w", err)
	}
	out.InitiatedBy = initiator
	// AuditEvent rather than Audit: a send being refused is an occurrence with
	// no prior state, and an update audit would demand a before-image that does
	// not exist. The addresses stay OFF the audit payload — they are already on
	// the row, and a second copy would outlive the erasure that clears the
	// first.
	if _, err := storekit.AuditEvent(ctx, tx, "create", "communication_review", out.ID, map[string]any{
		"reason_code": out.ReasonCode,
		"recipients":  len(refusals),
		"state":       out.State,
	}); err != nil {
		return Review{}, err
	}
	return out, nil
}

// ReviewForReader reads one review back for somebody entitled to see it.
//
// TWO DOORS, and they answer different questions.
//
// THE INITIATOR, because it is their message. They pressed send, they were
// refused, and the row is the record of what happened to them.
//
// THE EXCEPTION HOLDER, because they are the human being asked to decide it.
// Until this, a reviewer handed a card could not read the refusal behind it:
// the approve button worked and the thing it was about was a 404. They
// acknowledged a warning about a message they had never seen, which is the one
// thing an acknowledgement must not be.
//
// NOBODY ELSE. A review names the recipients of somebody's message and the
// reason each was refused, which is a fact about those contacts rather than about
// the sender — so a seat holding neither door sees nothing, and holding an
// unrelated grant admits nothing.
//
// A review the caller may not see answers NOT FOUND rather than forbidden, for
// the reason every scoped read here does: "forbidden" tells a caller the id
// exists, which is itself a disclosure about a message they may not see.
func (s *Store) ReviewForReader(ctx context.Context, id ids.UUID) (Review, error) {
	// A HUMAN. auth.RequireHuman admits connectors, which run with the
	// granting human's grants — so on the decider's door it would hand one
	// seat's refused correspondence to anything holding their credentials.
	// The initiator's own door is bounded by the seat either way.
	if err := requireAHumanAtTheKeyboard(ctx); err != nil {
		return Review{}, err
	}
	seat := initiatingSeat(ctx)
	// MAY THIS CALLER SEE REFUSED SENDS AT ALL. Asked once, here, rather than
	// folded into the query: a grant check that lived in SQL would be a second
	// place the RBAC answer is computed, and the two would disagree the first
	// time either changed.
	//
	// READ, not create. The two verbs on this object are different authorities:
	// create is who may act against the engine's answer about a contact, read is
	// who may SEE the queue of refusals. An installation can hand somebody the
	// reviewer's view without thereby letting them override anything, and
	// gating a read on the write grant would take that choice away.
	decider := auth.Require(ctx, entityCommunicationException, principal.ActionRead) == nil
	if seat.IsZero() && !decider {
		return Review{}, apperrors.ErrNotFound
	}
	var out Review
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var payload []byte
		err := tx.QueryRow(ctx, `
			SELECT id, state, kind, coalesce(delivery_intent_id, '00000000-0000-0000-0000-000000000000'::uuid),
			       refusals, reason_code,
			       coalesce(approval_id, '00000000-0000-0000-0000-000000000000'::uuid),
			       coalesce(initiated_by, '00000000-0000-0000-0000-000000000000'::uuid)
			  FROM communication_review
			 WHERE id = $1
			   AND (($2::uuid IS NOT NULL AND initiated_by = $2) OR $3)`, id, zeroSeatAsNull(seat), decider).
			Scan(&out.ID, &out.State, &out.Kind, &out.IntentID, &payload, &out.ReasonCode,
				&out.ApprovalID, &out.InitiatedBy)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("consent: reading the review: %w", err)
		}
		return json.Unmarshal(payload, &out.Refusals)
	})
	return out, err
}

// refusedRecipientsOf lifts the denied half of a decision set into what the row
// stores. Order follows the set, so the first refusal is the one the summary
// names.
func refusedRecipientsOf(set commsauthz.DecisionSet) []RefusedRecipient {
	denied := set.Denied()
	if len(denied) == 0 {
		return nil
	}
	out := make([]RefusedRecipient, 0, len(denied))
	for _, d := range denied {
		refusal := RefusedRecipient{
			// NORMALIZED ON THE WAY IN, because the erasure sweeps match on it.
			// An address stored with the whitespace or the casing a caller
			// typed is one a lowercased, trimmed comparison walks past — and
			// what it walks past is an erased subject's mailbox.
			Address:     strings.ToLower(strings.TrimSpace(d.Recipient.Email)),
			SubjectKind: d.SubjectKind,
			ReasonCode:  d.ReasonCode,
			Category:    string(d.Resolved),
		}
		if !d.SubjectID.IsZero() {
			refusal.SubjectID = d.SubjectID.String()
		}
		out = append(out, refusal)
	}
	return out
}

// stateFor decides which kind of work this refusal is.
//
// The question is whether MORE EVIDENCE could change the answer. A message
// refused for want of a ground can be answered by somebody saying what the
// ground is; a message refused because the subject asked us to stop, or because
// the address is dead, cannot — no context repairs those, and offering a
// context form for them would invite a rep to argue with a withdrawal.
func stateFor(refusals []RefusedRecipient) string {
	for _, r := range refusals {
		if !answerableByContext(r.ReasonCode) {
			return ReviewNeedsRepair
		}
	}
	return ReviewNeedsContext
}

// answerableByContext reports whether evidence could still make this send
// lawful.
//
// LISTED POSITIVELY rather than by exclusion. A reason code added later is not
// answerable until somebody decides it is, which fails toward the state that
// offers no argument — the safe direction, because the alternative is a form
// inviting a rep to argue with a withdrawal.
//
// The two here are the refusals about what the INSTALLATION knows: no evidence
// that this message has a ground to stand on, and a purpose key nothing
// defines. Somebody who was on the call can answer both.
//
// Everything else is about the SUBJECT or the ADDRESS and no context repairs
// it. An objection, a restriction, a withdrawal and a subject request are the
// subject's own instruction; a hard bounce is a dead mailbox; a frequency cap
// is a fact about volume that time answers rather than evidence.
func answerableByContext(reasonCode string) bool {
	switch reasonCode {
	case commsauthz.ReasonNoEvidence, commsauthz.ReasonUnknownPurpose,
		commsauthz.ReasonLegacyTransactionalUnevidenced:
		return true
	}
	return false
}

// initiatingSeat reads the human this send belongs to, or the zero id for a
// principal that is not a seat.
func initiatingSeat(ctx context.Context) ids.UUID {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return ids.UUID{}
	}
	return actor.UserID
}

// zeroUUIDAsNull sends a zero id as SQL NULL, so an absent intent or an absent
// seat is stored as absent rather than as an id that looks real.
func zeroUUIDAsNull(id ids.UUID) *ids.UUID {
	if id.IsZero() {
		return nil
	}
	return &id
}

// RecordRefusal opens a review for a refusal that is about to roll back its own
// transaction.
//
// A KNOWN RACE, stated rather than hidden: this write takes no subject lock, so
// a refusal committing while an erasure sweeps can land a review naming an
// address the sweep has already cleared.
//
// It is not closed by locking here. The erasure holds its address locks for the
// length of its own transaction, and this one begins after the send transaction
// has unwound — there is no moment when both are open for a lock to order. What
// would close it is recording the refusal inside the send's transaction, which
// is exactly what cannot happen: that transaction rolls back and takes the row
// with it.
//
// The exposure is bounded and the direction is the safe one. What survives is a
// snapshot of a message that was REFUSED — never sent, so nothing was disclosed
// — naming an address the installation was told to forget. It is cleared by the
// next erasure or anonymization of that subject, and until then it sits in a
// row only the initiator can read. The alternative, dropping the review when a
// refusal races a sweep, loses the record of a send that stopped.
//
// ITS OWN TRANSACTION, and that is forced rather than chosen. A staging refusal
// answers an error, and the caller returns it — which rolls back everything
// that transaction wrote, a review included. The record of what happened has to
// survive the rollback of the thing it is recording.
//
// The cost is honest and worth naming: this commits even though the send did
// not, so a crash between the two leaves a review for a message that never
// staged. That is the safe direction. A review with no message is visible work
// somebody can cancel; a message refused with no review is the silence this
// whole slice exists to end.
func (g *Gate) RecordRefusal(ctx context.Context, set commsauthz.DecisionSet, intentID ids.UUID) (Review, error) {
	var out Review
	err := g.store.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = OpenReviewTx(ctx, tx, set, intentID)
		return err
	})
	return out, err
}

// SendRefusedError is a refusal that left a review behind.
//
// It WRAPS the original refusal rather than replacing it, so every reader that
// already knows what apperrors.ErrConsentNotGranted means keeps working — the
// 409 mapping, the job-fault table, the tests. What it adds is the one thing
// the rep was missing: a reference to the record of what happened, which is how
// they get from "this was refused" to "here is what was refused and for whom".
type SendRefusedError struct {
	ReviewID ids.UUID
	Cause    error
	// Actions is what THIS caller may do about the refusal, decided where the
	// refusal is recorded because that is where the principal is known. Empty
	// for a caller with nothing to do but stop and report, which is the honest
	// answer for an agent: routing a decision is human-only and bound to the
	// review's initiator.
	Actions []string
}

func (e *SendRefusedError) Error() string {
	return e.Cause.Error() + " (review " + e.ReviewID.String() + ")"
}

// Unwrap keeps the refusal's own identity reachable, which is what lets the
// existing 409 mapping and every errors.Is on it go on working.
func (e *SendRefusedError) Unwrap() error { return e.Cause }

// NO FieldFault, deliberately, though every other typed refusal in this module
// carries one.
//
// A field fault answers 422 and names an input the caller should change. This
// refusal is neither: the caller's input was fine, the engine refused the send
// on consent grounds, and that answer is a 409 with the code every client and
// every test already recognises. Implementing FieldFault here changed the
// status from 409 to 422 across the send surface — the transport prefers a
// module's declared fault over a wrapped sentinel — which would have broken
// clients to deliver a reference.
//
// The reference travels in the message instead, which is where a human reads
// it and where the MCP surface renders it too.

// ReviewForInitiator reads one review back for the colleague who pressed send, and
// for nobody else.
//
// NARROWER THAN ReviewForReader ON PURPOSE. Reading a review is something a
// decider must be able to do — they are being asked about it. ROUTING one is
// not: a seat that could route anybody's review would be raising cards about
// other colleagues' correspondence, and an exception holder can already direct the
// send themselves rather than asking somebody to.
//
// So the two doors stay separate, and the narrow one is what routing uses.
func (s *Store) ReviewForInitiator(ctx context.Context, id ids.UUID) (Review, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return Review{}, err
	}
	seat := initiatingSeat(ctx)
	if seat.IsZero() {
		return Review{}, apperrors.ErrNotFound
	}
	var out Review
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var payload []byte
		err := tx.QueryRow(ctx, `
			SELECT id, state, kind, coalesce(delivery_intent_id, '00000000-0000-0000-0000-000000000000'::uuid),
			       refusals, reason_code
			  FROM communication_review
			 WHERE id = $1 AND initiated_by = $2`, id, seat).
			Scan(&out.ID, &out.State, &out.Kind, &out.IntentID, &payload, &out.ReasonCode)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("consent: reading the review: %w", err)
		}
		out.InitiatedBy = seat
		return json.Unmarshal(payload, &out.Refusals)
	})
	return out, err
}

// zeroSeatAsNull sends a seatless principal as SQL NULL.
//
// initiated_by is nullable — an erased or deleted seat leaves it so — and
// comparing it against the zero uuid would be comparing two absences. NULL = x
// is never true in Postgres, so this arm already matched nothing; sending NULL
// says that on purpose rather than relying on it.
func zeroSeatAsNull(seat ids.UUID) *ids.UUID {
	if seat.IsZero() {
		return nil
	}
	return &seat
}

// FaultReference implements apperrors.ReferencedFault: the review this refusal
// opened, as a field rather than as a sentence.
//
// STILL NO FieldFault, and the distinction matters. A field fault says "you
// sent a bad input" and answers 422; this refusal is neither — the caller's
// input was fine and the engine refused on consent grounds, which is the 409
// every client and every test already recognises. Implementing FieldFault here
// once flipped the whole send surface from 409 to 422.
//
// What a reference adds is orthogonal to that: the status and the code are
// unchanged, and the id appears beside them where a machine can read it. The
// tool surface is why — an agent handed a sentence can do nothing, and the same
// refusal naming its review can hand the question to a human.
func (e *SendRefusedError) FaultReference() map[string]any {
	if e.ReviewID.IsZero() {
		return nil
	}
	return map[string]any{
		FieldReviewID: e.ReviewID.String(),
		// WHAT THIS CALLER MAY DO, not what the route exists for.
		//
		// Routing a decision is human-only and bound to the review's own
		// initiator, so naming it unconditionally would promise an agent a move
		// it cannot make — and an agent that tries and is refused has been sent
		// somewhere by us rather than by its own mistake. Worse than saying
		// nothing.
		//
		// An empty list is the honest answer for a caller with nothing to do
		// but stop and report: the reference still travels, and a human
		// reading the agent's transcript can pick the review up.
		"available_actions": e.Actions,
	}
}

// Unreferenced answers the refusal without the reference, which is what decides
// the status. It is the wrapped cause rather than this value, or classification
// would come straight back here.
func (e *SendRefusedError) Unreferenced() error { return e.Cause }
