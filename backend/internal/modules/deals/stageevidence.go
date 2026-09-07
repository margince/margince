// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The evidence ledger: what was observed about a deal against one of its
// stage's exit criteria.
//
// Every writer here is DETERMINISTIC — it restates something a record already
// says. A contract turned active; a buyer confirmed a version in the deal
// room; a meeting was held with someone from their side in it. No model reads
// anything in this file, and author_side is computed by AuthorSideOf from the
// activity's own participants rather than claimed by whoever is writing.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const evidenceEntity = "deal_stage_evidence"

// ExtractedByDeterministic marks a claim no model was asked about. The CHECK
// on the table refuses a confidence beside it: a record either says the thing
// or it does not.
const ExtractedByDeterministic = "deterministic"

// The evidence source vocabulary, mirroring deal_stage_evidence.source_type.
//
// A deal-room decision is deliberately NOT here. The room is a place to
// discuss a document, not to accept one, and the event that once recorded an
// approval is retired — public-events.yaml marks it historical and nothing
// emits it. A source type no writer can produce is a vocabulary entry that
// only ever misleads a reader about what the ledger can hold.
const (
	SourceActivity = "activity"
	SourceContract = "contract"
)

// The commitment vocabulary, mirroring deal_stage_evidence.commitment.
//
// A deterministic writer only ever records CommitmentAgreed: it fires on a
// record that already settled — a signature, a confirmation, a meeting that
// happened. Distinguishing a proposal from an agreement is a reading task, and
// reading is the model's half of this ledger.
const (
	CommitmentAgreed   = "agreed"
	CommitmentProposed = "proposed"
	CommitmentNone     = "none"
)

// EvidenceInput is one observation about one criterion.
type EvidenceInput struct {
	DealID      ids.DealID
	CriterionID ids.ExitCriterionID
	SourceType  string
	SourceID    ids.UUID
	SourceLines []int32
	Snippet     *string
	AuthorSide  AuthorSide
	Commitment  string
	Met         bool
	// Confidence is nil for a deterministic writer and required for a model's
	// claim; the column's CHECK holds the first half.
	Confidence  *float64
	ObservedAt  time.Time
	ExtractedBy string
}

// RecordStageEvidence writes one observation, or leaves the existing one
// standing when this source has already been recorded against this criterion.
//
// Idempotent by (deal, criterion, source), because the bus is at-least-once:
// the same contract turning active is delivered more than once, and a second
// row would double-count the same fact for every reader that counts met
// criteria. The unique index is what holds it; this answers the conflict
// rather than surfacing a 23505.
func (s *Store) RecordStageEvidence(
	ctx context.Context, in EvidenceInput,
) (crmcontracts.StageEvidence, error) {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return crmcontracts.StageEvidence{}, err
	}
	if err := validEvidence(in); err != nil {
		return crmcontracts.StageEvidence{}, err
	}
	var out crmcontracts.StageEvidence
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		// EnsureWritableLive, not EnsureVisibleLive: a manual grant widens
		// VISIBILITY at either access level, so the weaker probe would let a
		// caller holding only a read share hang evidence on the deal — and
		// evidence is what a stage move will later rest on.
		if err := auth.EnsureWritableLive(ctx, tx, dealTable, in.DealID.UUID); err != nil {
			return err
		}
		if err := refuseUnsettleableCriterion(ctx, tx, in); err != nil {
			return err
		}
		id, written, err := insertEvidence(ctx, tx, in)
		if err != nil {
			return err
		}
		if out, err = readEvidence(ctx, tx, id); err != nil {
			return err
		}
		if !written {
			// A redelivery. The row already stands and was already audited, so
			// writing again would put a second create in the trail for one fact.
			return nil
		}
		return announceEvidence(ctx, tx, in.DealID, id, evidenceAfter(out))
	})
	return out, err
}

// RefuteStageEvidence marks a claim incorrect.
//
// The row STAYS. Why a stage move was reversed is a question asked later, and
// a deleted claim answers it with silence. Refuting is also not the same as
// rejecting a proposed move: a human may disagree with the move while
// accepting every fact under it, so the two verbs are separate.
func (s *Store) RefuteStageEvidence(
	ctx context.Context, dealID ids.DealID, id ids.UUID,
) (crmcontracts.StageEvidence, error) {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return crmcontracts.StageEvidence{}, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return crmcontracts.StageEvidence{}, apperrors.ErrPermissionDenied
	}
	var out crmcontracts.StageEvidence
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		// A refutation changes the ledger, so it takes the write probe too: a
		// read share must not be able to strike out somebody else's evidence.
		if err := auth.EnsureWritableLive(ctx, tx, dealTable, dealID.UUID); err != nil {
			return err
		}
		lock, err := storekit.LockRow(ctx, tx, evidenceEntity, id, storekit.IncludeArchived)
		if err != nil {
			return err
		}
		before, err := readEvidence(ctx, tx, id)
		if err != nil {
			return err
		}
		if before.DealId != openapiUUID(dealID.UUID) {
			return apperrors.ErrNotFound
		}
		if before.RefutedAt != nil {
			// Already refuted, by someone. Re-refuting would overwrite who
			// first said so, and the first word is the one worth keeping.
			out = before
			return nil
		}
		patch := storekit.NewPatch()
		patch.Set("refuted_at", nil, time.Now().UTC())
		patch.Set("refuted_by", nil, actor.UserID)
		if err := patch.ApplyLocked(ctx, tx, lock); err != nil {
			return fmt.Errorf("refute stage evidence: %w", err)
		}
		auditID, err := storekit.Audit(ctx, tx, "update", evidenceEntity, id,
			patch.Before(), patch.After())
		if err != nil {
			return fmt.Errorf("audit stage evidence refutation: %w", err)
		}
		if out, err = readEvidence(ctx, tx, id); err != nil {
			return err
		}
		return emitEvidenceChanged(ctx, tx, auditID, dealID)
	})
	return out, err
}

// ListStageEvidence answers what has been observed about a deal.
//
// Refuted rows are INCLUDED. A reader asking what a stage move rested on needs
// to see the claim that was withdrawn as much as the ones that stood, and the
// refuted_at column is what tells them apart.
func (s *Store) ListStageEvidence(
	ctx context.Context, dealID ids.DealID,
) ([]crmcontracts.StageEvidence, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []crmcontracts.StageEvidence
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		// Anything that returns a record is a read, so the deal carries the
		// row-scope gate before its evidence is handed back.
		//
		// EnsureVisibleLive, not EnsureVisible: this hands records back, and
		// the strict probe is the one that keeps the existence check for an
		// unbounded actor and refuses an archived deal. An erasure stamps
		// archived_at while leaving owner_id alone, so the weaker probe would
		// answer "still yours" for a deal every live read path refuses.
		if err := auth.EnsureVisibleLive(ctx, tx, dealTable, dealID.UUID); err != nil {
			return err
		}
		var err error
		out, err = readDealEvidence(ctx, tx, dealID)
		return err
	})
	return out, err
}

// insertEvidence writes the row, answering whether THIS call wrote it.
func insertEvidence(
	ctx context.Context, tx pgx.Tx, in EvidenceInput,
) (ids.UUID, bool, error) {
	var id ids.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO deal_stage_evidence (
			deal_id, criterion_id, source_type, source_id, source_lines, snippet,
			author_side, commitment, met, confidence, observed_at, extracted_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (deal_id, criterion_id, source_type, source_id) DO NOTHING
		RETURNING id`,
		in.DealID, in.CriterionID, in.SourceType, in.SourceID, in.SourceLines,
		in.Snippet, string(in.AuthorSide), in.Commitment, in.Met, in.Confidence,
		in.ObservedAt, in.ExtractedBy).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		// The conflict target matched: this source is already recorded against
		// this criterion. Answer the standing row rather than the new one.
		existing, err := evidenceIDFor(ctx, tx, in)
		return existing, false, err
	}
	if err != nil {
		return id, false, fmt.Errorf("insert stage evidence: %w", err)
	}
	return id, true, nil
}
