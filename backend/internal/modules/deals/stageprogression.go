// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The proposal a stage decision becomes, and the ledger of how it was received.
//
// The staging itself belongs to the approvals module, which this one may not
// import, so the seam is a function the composition root supplies. What lives
// here is the payload's shape, the outcome vocabulary and the writes — the
// facts a deal's own module owns.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// StageProgressionKind is the approval kind a proposed move is staged as.
const StageProgressionKind = "stage_progression"

const progressionEntity = "stage_progression_outcome"

// The two audit-image keys this file writes. Constants rather than literals at
// each site, so a typo is a compile error instead of a field nobody reads
// sitting in the trail beside the one somebody expected.
const (
	progressionOutcomeKey = "outcome"
	progressionDealKey    = "deal_id"
)

// The outcome vocabulary, mirroring stage_progression_outcome.outcome.
//
// Proposed is written when the card is staged, and updated in place when it is
// decided — one row per approval, because a proposal is received once and a
// second row would double-count it in every rate the report computes.
const (
	ProgressionProposed       = "proposed"
	ProgressionApprovedClean  = "approved_clean"
	ProgressionApprovedEdited = "approved_edited"
	ProgressionRejected       = "rejected"
	ProgressionAutoApplied    = "auto_applied"
	ProgressionReversed       = "reversed"
	ProgressionExpired        = "expired"
	// ProgressionSuperseded closes the row of a card a FRESHER reading
	// replaced. Its own outcome rather than an expiry: nobody declined to
	// answer it, a better question simply arrived, and counting it as
	// unanswered would read as the product asking things nobody looks at.
	ProgressionSuperseded = "superseded"
)

// StageProgressionChange is the proposal a human reads and decides on.
//
// The criteria travel WITH the proposal rather than being re-read at decision
// time. A rep confirms the card they were shown, and re-reading the ledger at
// acceptance would let evidence recorded in between change what they agreed to.
type StageProgressionChange struct {
	DealID      ids.DealID          `json:"deal_id"`
	FromStageID ids.StageID         `json:"from_stage_id"`
	ToStageID   ids.StageID         `json:"to_stage_id"`
	FromName    string              `json:"from_stage_name"`
	ToName      string              `json:"to_stage_name"`
	Criteria    []ProposedCriterion `json:"criteria"`
	// Because is the product's own sentence about why this is being asked. It
	// leads the card, because a rep deciding needs the reason before the
	// checklist.
	Because string `json:"because"`
	// ConfirmFirst marks a move that needs a judgement rather than an assent —
	// a closing stage, a skip, a criterion resting on something proposed. The
	// card says so out loud instead of looking like every other card.
	ConfirmFirst bool `json:"confirm_first"`
	// WonWithoutContractReason answers the paperless-win bound when this move
	// lands on a won stage: a win with no agreement behind it is refused
	// unless somebody says why.
	//
	// It is the APPROVER's to supply, through the card's editable field.
	// Nothing the product read can answer why a deal was won without paper —
	// that is an argument a person makes, and a proposer filling it in would be
	// inventing the argument the bound exists to demand.
	//
	// PRESENT AND EMPTY on such a proposal, never absent, and the difference is
	// the whole mechanism: the editor skips a declared field the payload does
	// not carry, because adding a path on approve reads as a retargeted edit.
	// An omitted key would therefore render no control at all, and the card
	// would be one a rep could approve and approve and never move the deal.
	WonWithoutContractReason *string `json:"won_without_contract_reason,omitempty"`
	WonWithoutContractDetail *string `json:"won_without_contract_detail,omitempty"`
}

// ProposedCriterion is one criterion as the card shows it.
type ProposedCriterion struct {
	CriterionID ids.ExitCriterionID `json:"criterion_id"`
	Key         string              `json:"key"`
	Label       string              `json:"label"`
	Required    bool                `json:"required"`
	Met         bool                `json:"met"`
	// EvidenceIDs are the ledger rows behind it, so a reviewer can open what
	// the claim rests on rather than taking the tick on trust.
	EvidenceIDs []ids.UUID `json:"evidence_ids"`
}

// StageProgressionIdentity is what makes a fresher read supersede a stale card
// rather than compete with it in the inbox.
//
// The TARGET STAGE alone. Two readings of one deal proposing the same move are
// one question asked twice, however much the evidence beneath them changed —
// and approving the stale one after the fresh one would move the deal on a
// checklist nobody is looking at any more.
type StageProgressionIdentity struct {
	ToStageID ids.StageID `json:"to_stage_id"`
}

// ProgressionOutcomeInput is one decision, as the ledger records it.
type ProgressionOutcomeInput struct {
	ApprovalID  ids.UUID
	DealID      ids.DealID
	PipelineID  ids.PipelineID
	FromStageID ids.StageID
	ToStageID   ids.StageID
	// EvidenceKinds are the criterion kinds the move rested on, so the report
	// can answer which evidence this installation actually accepts.
	EvidenceKinds []string
	Outcome       string
	// RejectionReason is the rep's own words, and only a rejection carries one.
	RejectionReason *string
	// DecidedBySystem marks a move nobody was asked about.
	DecidedBySystem bool
}

// RecordProgressionProposed opens the ledger row for a staged proposal.
//
// Written at STAGING rather than at decision, so a proposal nobody ever
// answers is still countable: an expired card is a real outcome — the product
// asked and the rep did not think it worth answering — and a ledger that only
// recorded decisions would report a clean acceptance rate over the subset
// somebody bothered with.
func (s *Store) RecordProgressionProposed(
	ctx context.Context, tx pgx.Tx, in ProgressionOutcomeInput,
) error {
	// The gate applies here as much as on the decision: opening a ledger row
	// for a deal is a write about that deal, and a caller who may not move it
	// may not put a proposal to move it on the record either.
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return err
	}
	if in.Outcome == "" {
		in.Outcome = ProgressionProposed
	}
	if err := validProgressionOutcome(in); err != nil {
		return err
	}
	var id ids.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO stage_progression_outcome (
			approval_id, deal_id, pipeline_id, from_stage_id, to_stage_id,
			evidence_kinds, outcome, decided_by_system)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (approval_id) DO NOTHING
		RETURNING id`,
		in.ApprovalID, in.DealID, in.PipelineID, in.FromStageID, in.ToStageID,
		in.EvidenceKinds, in.Outcome, in.DecidedBySystem).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		// The proposal was already recorded. Staging is at-least-once through
		// JoinPending, and a second row for one approval would double-count it.
		return nil
	}
	if err != nil {
		return fmt.Errorf("record the proposed stage move: %w", err)
	}
	auditID, err := storekit.AuditEvent(ctx, tx, "create", progressionEntity, id,
		progressionAfter(in))
	if err != nil {
		return fmt.Errorf("audit the proposed stage move: %w", err)
	}
	if err := supersedeOpenProgressions(ctx, tx, in, id); err != nil {
		return err
	}
	return emitProgressionChanged(ctx, tx, auditID, in.DealID)
}

// supersedeOpenProgressions closes the rows of any card this proposal replaced.
//
// Staging supersedes a stale card under the same identity, and it expires that
// approval WITHOUT emitting approval.decided — the outcome consumer never hears
// about it. So the row would stand at `proposed` forever, and every rate the
// launch gate computes would be taken over a denominator that keeps growing
// with rows nothing can ever close.
//
// Closed HERE, in the transaction that opens the replacement, because that is
// the moment the replacement is a fact. Doing it from the consumer would need
// an event nothing emits.
func supersedeOpenProgressions(
	ctx context.Context, tx pgx.Tx, in ProgressionOutcomeInput, survivor ids.UUID,
) error {
	rows, err := tx.Query(ctx, `
		UPDATE stage_progression_outcome
		   SET outcome = $4, decided_at = now()
		 WHERE deal_id = $1 AND to_stage_id = $2 AND outcome = $5
		   AND id <> $3
		RETURNING id`,
		in.DealID, in.ToStageID, survivor, ProgressionSuperseded, ProgressionProposed)
	if err != nil {
		return fmt.Errorf("close the stage moves this proposal replaced: %w", err)
	}
	var closed []ids.UUID
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("scan a superseded stage move: %w", err)
		}
		closed = append(closed, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read the superseded stage moves: %w", err)
	}
	for _, id := range closed {
		auditID, err := storekit.Audit(ctx, tx, "update", progressionEntity, id,
			map[string]any{progressionOutcomeKey: ProgressionProposed},
			map[string]any{
				progressionOutcomeKey: ProgressionSuperseded,
				"superseded_by":       survivor.String(),
			})
		if err != nil {
			return fmt.Errorf("audit a superseded stage move: %w", err)
		}
		// Every audited mutation ships its event. A closed row changes what the
		// stage-automation report answers for this deal, and a consumer that
		// heard the proposal open must hear it close.
		if err := emitProgressionChanged(ctx, tx, auditID, in.DealID); err != nil {
			return err
		}
	}
	return nil
}

// RecordProgressionDecided closes the ledger row for a proposal that was
// answered — accepted, rejected, applied by the system, or left to expire.
//
// It UPDATES the proposed row rather than inserting a second one, so the
// report counts one proposal once. A decision arriving for a proposal nobody
// recorded is answered rather than ignored: the ledger is what the launch gate
// reads, and a silent gap in it would be measured as good behaviour.
func (s *Store) RecordProgressionDecided(
	ctx context.Context, approvalID ids.UUID, outcome string, reason *string, edited bool,
) error {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return err
	}
	return s.Tx(ctx, func(tx pgx.Tx) error {
		return recordProgressionDecidedTx(ctx, tx, approvalID, outcome, reason, edited)
	})
}

func recordProgressionDecidedTx(
	ctx context.Context, tx pgx.Tx, approvalID ids.UUID,
	outcome string, reason *string, edited bool,
) error {
	if !progressionOutcomes[outcome] {
		return fmt.Errorf("deals: %q is not a stage progression outcome", outcome)
	}
	if reason != nil && outcome != ProgressionRejected {
		// The column's CHECK refuses this too. Answering it here says which of
		// the two the caller should change.
		return fmt.Errorf("deals: only a rejection carries a reason, not %q", outcome)
	}
	var id ids.UUID
	var dealID ids.DealID
	err := tx.QueryRow(ctx, `
		UPDATE stage_progression_outcome
		   SET outcome = $2, rejection_reason = $3, evidence_corrected = $4,
		       decided_at = now()
		 WHERE approval_id = $1 AND outcome = $5
		RETURNING id, deal_id`,
		approvalID, outcome, reason, edited, ProgressionProposed).
		Scan(&id, &dealID)
	if errors.Is(err, pgx.ErrNoRows) {
		// Either nothing was proposed under this approval, or it was already
		// decided. Both are ordinary on an at-least-once path — a redelivered
		// decision must not overwrite the first one, which is the one that
		// happened.
		return nil
	}
	if err != nil {
		return fmt.Errorf("record the stage move's outcome: %w", err)
	}
	// The BEFORE image is known without reading it back: the WHERE clause
	// admitted this row only while it stood at proposed, and the insert writes
	// evidence_corrected false. A reader of the trail then sees a proposal
	// becoming a rejection rather than a rejection appearing from nowhere.
	//
	// Not taken from RETURNING, which answers the row AFTER the update and
	// would make the before-image a copy of the after one.
	auditID, err := storekit.Audit(ctx, tx, "update", progressionEntity, id,
		map[string]any{progressionOutcomeKey: ProgressionProposed, "evidence_corrected": false},
		map[string]any{progressionOutcomeKey: outcome, "evidence_corrected": edited})
	if err != nil {
		return fmt.Errorf("audit the stage move's outcome: %w", err)
	}
	return emitProgressionChanged(ctx, tx, auditID, dealID)
}

// progressionOutcomes is the vocabulary the column's CHECK holds, spelled here
// so a caller's typo is a refusal naming the field rather than a 23514.
var progressionOutcomes = map[string]bool{
	ProgressionProposed: true, ProgressionApprovedClean: true,
	ProgressionApprovedEdited: true, ProgressionRejected: true,
	ProgressionAutoApplied: true, ProgressionReversed: true,
	ProgressionExpired: true, ProgressionSuperseded: true,
}

func validProgressionOutcome(in ProgressionOutcomeInput) error {
	if !progressionOutcomes[in.Outcome] {
		return fmt.Errorf("deals: %q is not a stage progression outcome", in.Outcome)
	}
	if in.ApprovalID == (ids.UUID{}) {
		return errors.New("deals: a progression outcome names the approval it records")
	}
	return nil
}

// progressionAfter is the audit after-image. The transition and the outcome
// are what a later reader judges the row by, so both are named.
func progressionAfter(in ProgressionOutcomeInput) map[string]any {
	return map[string]any{
		"approval_id": in.ApprovalID.String(), progressionDealKey: in.DealID.String(),
		"from_stage_id": in.FromStageID.String(), "to_stage_id": in.ToStageID.String(),
		progressionOutcomeKey: in.Outcome, "decided_by_system": in.DecidedBySystem,
	}
}

// emitProgressionChanged publishes the one fact a progression write makes:
// what is proposed or decided about this deal's stage changed.
func emitProgressionChanged(
	ctx context.Context, tx pgx.Tx, auditID ids.UUID, dealID ids.DealID,
) error {
	if err := storekit.EmitEvent(ctx, tx, auditID, dealID.UUID,
		crmcontracts.PublicEventDealUpdated{
			ChangedFields: map[string]any{"stage_progression": true},
		}); err != nil {
		return fmt.Errorf("emit deal.updated for its stage progression: %w", err)
	}
	return nil
}

// ReadProgressionChange decodes a staged proposal's payload.
//
// A payload that cannot be read is a card a human may have already seen, so it
// answers a refusal naming the approval rather than a decoded zero value — a
// zero would move a deal to the zero stage.
func ReadProgressionChange(raw json.RawMessage) (StageProgressionChange, error) {
	var out StageProgressionChange
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("read the proposed stage move: %w", err)
	}
	if out.DealID == (ids.DealID{}) || out.ToStageID == (ids.StageID{}) {
		return out, apperrors.ErrNotFound
	}
	return out, nil
}
