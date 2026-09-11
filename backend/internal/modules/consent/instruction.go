// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// A named human deciding one refused message goes out anyway.
//
// The engine refuses and records why. Sometimes the installation has a reason
// the engine cannot see — a contract clause, a legal obligation, a subject who
// asked in a room nobody logged — and somebody with the authority decides to
// send.
//
// THIS IS NOT A CONSENT GRANT AND IS NEVER RECORDED AS ONE. The refusal stays
// exactly where it is; this row sits beside it saying a contact overrode it, who
// they were, and what they said their reason was. A subject asking later why
// they received a message must be shown the refusal AND the decision, not a
// grant nobody made.
//
// WHAT THIS SLICE DOES NOT DO. Nothing executes on an instruction yet. The row
// can be written and revoked and that is all — the send path that consumes one
// is its own change, and landing the record first means the authority question
// is settled before anything can act on the answer.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// entityCommunicationException is the RBAC object directing a send answers to.
//
// Its own object rather than a corner of consent_config, because the two are
// different authorities: consent_config is who may change the RULES, and this
// is who may act against the answer the rules produced about one contact. An
// installation that delegates the first has not thereby delegated the second.
const entityCommunicationException = "communication_exception"

// EntityCommunicationException is the same object, exported because compose
// decides which action to OFFER a refused caller and that decision turns on
// this grant. Two spellings would let the offer and the door disagree about
// who may direct a send.
const EntityCommunicationException = entityCommunicationException

// The reasons a director may give. CLOSED, because the row is read in an audit
// and a free-text-only reason cannot be counted — an installation asking "how
// often do we override, and for what" needs an answer it can group.
//
// `other` is present deliberately: a closed list with no escape makes contacts
// pick the nearest wrong entry, which is worse than one honest bucket whose
// explanation carries the meaning.
const (
	ReasonCustomerRequestedOutsideCRM = "customer_requested_outside_crm"
	ReasonContractualNecessity        = "contractual_necessity"
	ReasonLegalObligation             = "legal_obligation"
	ReasonOtherException              = "other"
)

// The statuses an instruction moves through.
const (
	InstructionDirected = "directed"
	InstructionConsumed = "consumed"
	InstructionRevoked  = "revoked"
	InstructionExpired  = "expired"
)

// maxExplanationRunes bounds what a director types. Long enough for the reason
// that matters, short enough that the field is not a second place to keep a
// conversation.
const maxExplanationRunes = 1000

// fieldReasonCode is the wire and audit name for which kind of reason a
// decision carries. One spelling, because a client tells one refusal from
// another on the exact string and an audit reader groups on it.
const fieldReasonCode = "reason_code"

// InstructionValidity is how long a decision stands before the facts under it
// are too old to act on.
//
// Twenty-four hours, and the number is a judgement rather than a constant with
// a derivation: long enough that a decision taken at the end of a day is still
// good the next morning, short enough that a message directed last week does
// not go out against a stop recorded since. The send path re-checks the facts
// regardless; this is the outer bound on how stale they may be.
const InstructionValidity = 24 * time.Hour

// DirectInput is what the contact directing the send says.
type DirectInput struct {
	ReasonCode  string
	Explanation string
	// WarningVersion is the compliance text they were shown. Recorded because a
	// record naming no version cannot say what they were told, and the text
	// changes.
	WarningVersion string
	// Acknowledged is the tick. Refused when false: the acknowledgement is the
	// act, and an instruction written without one would record a decision
	// nobody made.
	Acknowledged bool
}

// Instruction is one recorded decision.
type Instruction struct {
	ID         ids.UUID
	ReviewID   ids.UUID
	DirectedBy ids.UUID
	ReasonCode string
	Status     string
	ValidUntil time.Time
}

// DirectSend records that a named human decided this refused message goes.
//
// GATED ON THREE THINGS, and each answers a different question.
//
// RequireHuman, because the claim is that a CONTACT took responsibility. An
// agent acting under somebody's passport inherits their grants, so without this
// an agent could mint the very record that says a human decided — which is the
// one assertion this table exists to make truthfully.
//
// auth.Require on communication_exception:create, because directing a send is
// its own authority. A seat that may edit a contact has not thereby been given
// the right to act against the engine's answer about them.
//
// And the review's own state, because an instruction against a review that is
// already resolved, superseded or answered is a decision about a message that
// is no longer waiting on one.
func (s *Store) DirectSend(ctx context.Context, reviewID ids.UUID, in DirectInput) (Instruction, error) {
	if err := requireAHumanAtTheKeyboard(ctx); err != nil {
		return Instruction{}, err
	}
	if err := auth.Require(ctx, entityCommunicationException, principal.ActionCreate); err != nil {
		return Instruction{}, err
	}
	// NORMALISED BEFORE ANYTHING READS IT, so the value checked and the value
	// stored are one value. Trimming only inside the check let " override-v1 "
	// pass and then land on the row naming a version nothing serves.
	in.WarningVersion = strings.TrimSpace(in.WarningVersion)
	in.Explanation = strings.TrimSpace(in.Explanation)
	if err := validateDirect(in); err != nil {
		return Instruction{}, err
	}
	director := initiatingSeat(ctx)
	if director.IsZero() {
		return Instruction{}, apperrors.ErrPermissionDenied
	}
	var out Instruction
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		review, err := claimReviewForDirectionTx(ctx, tx, reviewID)
		if err != nil {
			return err
		}
		// THE FACTS ARE THE REVIEW'S, not the clock's. An instruction says "on
		// what this review recorded, send anyway", and dating it from now would
		// claim it was taken on facts nobody has looked at.
		out = Instruction{
			ReviewID:   reviewID,
			DirectedBy: director,
			ReasonCode: in.ReasonCode,
			Status:     InstructionDirected,
			ValidUntil: time.Now().Add(InstructionValidity),
		}
		// THE MESSAGE THEY READ, fingerprinted now. The held row this review
		// names can be edited afterwards, so a check made later against that
		// row would compare the message with itself.
		wording, known, err := acknowledgedWordingTx(ctx, tx, review.IntentID)
		if err != nil {
			return err
		}
		// A review with no readable held message records no fingerprint rather
		// than a zero one: an all-zero digest would later compare unequal to
		// every real message and refuse a send nobody can fix.
		var acknowledged []byte
		if known {
			acknowledged = wording[:]
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO communication_instruction
			  (review_id, directed_by, reason_code, explanation, warning_version,
			   acknowledged_at, facts_as_of, valid_until, acknowledged_wording)
			VALUES ($1, $2, $3, $4, $5, now(), $6, $7, $8)
			RETURNING id`,
			reviewID, director, in.ReasonCode, in.Explanation,
			in.WarningVersion, review.OpenedAt, out.ValidUntil, acknowledged).Scan(&out.ID); err != nil {
			// A DECISION ALREADY STANDS ON THIS REVIEW, which is what the
			// unique key on review_id says. Named rather than reported as a
			// storage fault: the caller retrying a send whose decision already
			// committed needs to tell this apart from a real failure, or a
			// transient hiccup in the send would trap the reviewer behind their
			// own record.
			if isDuplicateInstruction(err) {
				return ErrDecisionAlreadyRecorded
			}
			return fmt.Errorf("consent: recording the decision to send this refused message: %w", err)
		}
		// AuditEvent, not Audit: an instruction being given is an occurrence
		// with no prior state — the row did not exist a moment ago. It carries
		// the reason CODE and the warning version, never the explanation: that
		// is on the row, which an erasure reaches, and a second copy in the
		// audit trail would outlive it.
		if _, err := storekit.AuditEvent(ctx, tx, "create", "communication_instruction", out.ID,
			map[string]any{
				"review_id":       reviewID,
				fieldReasonCode:   in.ReasonCode,
				"warning_version": in.WarningVersion,
				// WHICH GRANT ADMITTED THIS, named on the payload because the
				// audit's own rule string is derived from the ENTITY and would
				// read `communication_instruction.create` — a permission
				// nothing checks. The entity stays the table, which is how a
				// reader finds the row; the authority is recorded here.
				"authorized_by": entityCommunicationException,
			}); err != nil {
			return err
		}
		return nil
	})
	return out, err
}

// RevokeInstruction takes a decision back before anything acts on it.
//
// GATED ON delete, which is the verb for undoing an authority rather than
// exercising one. A seat that may direct a send is not thereby the seat that
// may cancel somebody else's direction, and an installation can grant the two
// separately.
//
// A CONSUMED INSTRUCTION IS NOT REVOCABLE. The message has gone; taking the
// decision back would leave a sent message with no recorded authority behind
// it, which is worse than the decision standing.
func (s *Store) RevokeInstruction(ctx context.Context, id ids.UUID, reason string) error {
	if err := requireAHumanAtTheKeyboard(ctx); err != nil {
		return err
	}
	if err := auth.Require(ctx, entityCommunicationException, principal.ActionDelete); err != nil {
		return err
	}
	if strings.TrimSpace(reason) == "" {
		return &ValidationError{
			Field:  fieldReason,
			Reason: "say why the decision is being taken back",
		}
	}
	by := initiatingSeat(ctx)
	if by.IsZero() {
		return apperrors.ErrPermissionDenied
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE communication_instruction
			   SET status = 'revoked', revoked_at = now(), revoked_by = $2, revoked_reason = $3
			 WHERE id = $1 AND status = 'directed'`,
			id, by, strings.TrimSpace(reason))
		if err != nil {
			return fmt.Errorf("consent: taking back the decision to send: %w", err)
		}
		if tag.RowsAffected() == 0 {
			// Absent, already consumed, already revoked or expired. Answered as
			// not-found rather than as a conflict: a caller who may not see
			// which of those it is learns nothing from the difference, and a
			// consumed instruction is not theirs to undo either way.
			return apperrors.ErrNotFound
		}
		_, err = storekit.Audit(ctx, tx, "update", "communication_instruction", id,
			map[string]any{"status": InstructionDirected},
			map[string]any{
				"status":        InstructionRevoked,
				"authorized_by": entityCommunicationException,
			})
		return err
	})
}

// validateDirect refuses what the store cannot stand behind.
func validateDirect(in DirectInput) error {
	// THE ACKNOWLEDGEMENT IS THE ACT. Everything else on this row describes a
	// decision; this is the decision. Writing one without it would record that
	// somebody accepted responsibility they never accepted.
	if !in.Acknowledged {
		return &ValidationError{
			Field:  "acknowledged",
			Reason: "directing a send is an acknowledgement, and it has to be given",
		}
	}
	switch in.ReasonCode {
	case ReasonCustomerRequestedOutsideCRM, ReasonContractualNecessity,
		ReasonLegalObligation, ReasonOtherException:
	default:
		return &ValidationError{
			Field:  fieldReasonCode,
			Reason: "say which kind of reason this is",
		}
	}
	if strings.TrimSpace(in.Explanation) == "" {
		return &ValidationError{
			Field:  "explanation",
			Reason: "say why this message goes out despite the refusal",
		}
	}
	if len([]rune(in.Explanation)) > maxExplanationRunes {
		return &ValidationError{
			Field:  "explanation",
			Reason: "an explanation is at most 1000 characters",
		}
	}
	// A VERSION THIS BUILD ACTUALLY SERVES, not merely a non-empty string.
	//
	// The record's whole claim is that a named human read particular words. A
	// caller free to name any version could write "v99" onto an instruction and
	// the record would assert an acknowledgement of text nobody ever wrote —
	// which is exactly the assertion a dispute about an override turns on.
	//
	// The queue card has its own version because a card and a modal are not the
	// same words: somebody approving from a list read a summary, and recording
	// them as having read the modal's caution would overstate what they saw.
	// COMPARED AND STORED AS THE SAME VALUE. Trimming only for the comparison
	// let " override-v1 " pass the check and land on the row, where it names a
	// version the served set does not contain — so the record would claim an
	// acknowledgement of a version nothing publishes.
	if !servedWarningVersion(in.WarningVersion) {
		return &ValidationError{
			Field: "warning_version",
			Reason: "a decision records which warning the director was shown, and it must be one " +
				"this installation serves",
		}
	}
	return nil
}

// directableReview is what the direction reads off the review it answers.
type directableReview struct {
	State    string
	OpenedAt time.Time
	// IntentID is the held message this review is about, so the decision can
	// fingerprint what the human is looking at.
	IntentID ids.UUID
}

// claimReviewForDirectionTx takes the review this decision is about.
//
// NOT SCOPED TO THE INITIATOR, unlike every other read of this row, and the
// difference is the point: directing a send is precisely the act of somebody
// OTHER than the sender deciding. The authority is the grant checked above, and
// scoping to the seat that opened the review would make the reviewer path
// impossible.
func claimReviewForDirectionTx(ctx context.Context, tx pgx.Tx, id ids.UUID) (directableReview, error) {
	var out directableReview
	err := tx.QueryRow(ctx, `
		SELECT state, opened_at,
		       coalesce(delivery_intent_id, '00000000-0000-0000-0000-000000000000'::uuid)
		  FROM communication_review
		 WHERE id = $1 AND resolved_at IS NULL
		 FOR UPDATE`, id).Scan(&out.State, &out.OpenedAt, &out.IntentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return directableReview{}, apperrors.ErrNotFound
	}
	if err != nil {
		return directableReview{}, fmt.Errorf("consent: reading the review this decision answers: %w", err)
	}
	return out, nil
}

// requireAHumanAtTheKeyboard admits a HUMAN and nothing else.
//
// auth.RequireHuman is not enough here, and the gap is exact: it refuses buyers
// and agents, and ADMITS a connector. A connector runs with the granting
// human's UserID and their whole permission set (capture/registry.go), so an
// integration would mint a row saying that contact decided to send a refused
// message — attributing an override to somebody who was not there.
//
// Every other human-only surface can live with that, because a connector
// acting under somebody's authority is doing what they configured it to do.
// This one cannot: the row's entire content is the claim that a named contact
// took responsibility, and a claim like that must be true of the moment it
// records.
func requireAHumanAtTheKeyboard(ctx context.Context) error {
	if err := auth.RequireHuman(ctx); err != nil {
		return err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return fmt.Errorf("directing a send is a contact's own decision: %w",
			apperrors.ErrPermissionDenied)
	}
	return nil
}

// ErrDecisionAlreadyRecorded reports that this review already carries a
// decision. It is not a failure of the act — somebody decided, and the record
// of it stands — so a caller whose send failed after the decision committed
// treats it as "already done" and goes on to the send.
var ErrDecisionAlreadyRecorded = errors.New("consent: a decision already stands on this review")

// isDuplicateInstruction recognises the unique key on review_id, which is what
// makes one review carry one decision.
func isDuplicateInstruction(err error) bool {
	constraint, ok := storekit.UniqueViolation(err)
	return ok && strings.Contains(constraint, "review_id")
}

// servedWarningVersion reports whether this build published the wording a
// caller says they were shown.
//
// TWO, and they are different surfaces rather than two spellings of one. The
// override warning is the caution in the modal a director reads before sending;
// the queue-card version names the summary an approver answered from a list.
// Recording one as the other would say somebody read words they never saw.
func servedWarningVersion(version string) bool {
	switch strings.TrimSpace(version) {
	case OverrideWarningVersion, QueueCardWarningVersion:
		return true
	default:
		return false
	}
}

// QueueCardWarningVersion names the summary an approver answers from in the
// approvals queue, as distinct from the modal's own caution.
//
// It lives here rather than in compose so the validator and the effect that
// writes it read one constant: two spellings would let a version be recorded
// that the check does not admit.
const QueueCardWarningVersion = "queue-card-v1"
