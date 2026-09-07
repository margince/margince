// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The reads, guards and audit shaping behind stageevidence.go's three entry
// points.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// evidenceColumns is the SELECT list both evidence reads build from, so
// scanEvidence's fixed Scan order stays correct for each: a column added here
// without a matching dest fails to compile.
const evidenceColumns = `id, deal_id, criterion_id, source_type, source_id,
	source_lines, snippet, author_side, commitment, met, confidence,
	observed_at, extracted_by, refuted_at, refuted_by, created_at, updated_at,
	version`

const evidenceField = "evidence"

// validEvidence refuses what the columns' CHECKs would refuse, naming the
// field rather than letting a constraint violation surface as a 500.
func validEvidence(in EvidenceInput) error {
	switch in.SourceType {
	case SourceActivity, SourceContract:
	default:
		return &values.ParseError{
			Field: "source_type", Code: "invalid_evidence_source",
			Message: "source_type is one of activity, contract",
		}
	}
	switch in.Commitment {
	case CommitmentAgreed, CommitmentProposed, CommitmentNone:
	default:
		return &values.ParseError{
			Field: "commitment", Code: "invalid_evidence_commitment",
			Message: "commitment is one of agreed, proposed, none",
		}
	}
	if in.ExtractedBy == "" {
		return &values.ParseError{
			Field: "extracted_by", Code: "evidence_writer_unnamed",
			Message: "evidence names the writer that made the claim",
		}
	}
	// The table refuses this pairing too. Answering it here says which of the
	// two the caller should change.
	if in.ExtractedBy == ExtractedByDeterministic && in.Confidence != nil {
		return &values.ParseError{
			Field: "confidence", Code: "deterministic_evidence_is_certain",
			Message: "a deterministic writer restates what a record says, so it carries no confidence",
		}
	}
	if in.ObservedAt.IsZero() {
		return &values.ParseError{
			Field: "observed_at", Code: "evidence_unplaced_in_time",
			Message: "evidence says when it was observed",
		}
	}
	return nil
}

// refuseUnsettleableCriterion checks the two things that make an observation
// admissible, both at the WRITE rather than filtered at the read: a stored row
// carrying a false fact is one every later reader — the policy function, the
// approval card, the report — would have to remember to discount.
//
// FIRST, the criterion is on this deal's own stage. That holds whether or not
// the claim says met, because evidence against another deal's criterion is
// misfiled either way.
//
// SECOND, the rule the ledger exists for: a criterion naming something the
// BUYER did is not settled by evidence our own side authored. A rep writing
// "they confirmed the budget" is a rep's assertion, and a stage that advanced
// on it advanced on nothing.
//
// Evidence that the criterion was NOT met is exempt from the second: "the
// buyer has not confirmed" is an observation anybody may make, and refusing it
// would leave the ledger able to record only good news.
func refuseUnsettleableCriterion(ctx context.Context, tx pgx.Tx, in EvidenceInput) error {
	kind, err := criterionKindOnDealsStage(ctx, tx, in.DealID, in.CriterionID)
	if err != nil {
		return err
	}
	if !in.Met {
		return nil
	}
	if !SettlesBuyerMilestone(CriterionKind(kind), in.AuthorSide) {
		return &values.ParseError{
			Field: evidenceField, Code: "buyer_milestone_needs_buyer_evidence",
			Message: "this criterion names something the buyer did, so our own side's word does not settle it",
		}
	}
	return nil
}

// criterionKindOnDealsStage answers a criterion's kind, refusing one that
// belongs to no stage this deal has ever been on.
//
// The BINDING is the point, not the kind. Nothing in the table ties an evidence
// row's criterion to its deal — the two are separate foreign keys — so without
// this check a caller could hang evidence for one deal on another deal's
// criterion, and every reader that counts a stage's met criteria would count a
// row belonging to somebody else's stage. The unique index would not catch it:
// (deal, criterion, source) is unique whether or not the two agree.
//
// EVERY stage in the deal's history, not just its current one. Evidence is
// anchored to when the thing happened, so a signature observed while the deal
// was in Discovery settles a Discovery criterion even once the deal has moved
// on — and a check against the current stage alone would refuse exactly the
// rows criteriaOfKind is built to write. Both halves read which stages count
// from deal_stage_history, which is why a criterion criteriaOfKind resolved is
// one this check admits.
//
// Held by: TestEvidenceLandsOnTheStageTheDealWasOnWhenItHappened and
// TestEvidenceOlderThanTheDealLandsOnTheStageItWasCreatedOn
// (backend/internal/modules/deals/stageevidence_integration_test.go), which
// both fail if the two halves stop agreeing.
//
// ErrNotFound rather than a field fault: naming the mismatch would confirm
// that another stage's criterion exists.
func criterionKindOnDealsStage(
	ctx context.Context, tx pgx.Tx, dealID ids.DealID, criterionID ids.ExitCriterionID,
) (string, error) {
	var kind string
	err := tx.QueryRow(ctx, `
		SELECT c.kind
		  FROM stage_exit_criterion c
		 WHERE c.id = $1
		   AND c.stage_id IN (
			   SELECT h.to_stage_id FROM deal_stage_history h WHERE h.deal_id = $2
		   )`, criterionID, dealID).Scan(&kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperrors.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read the criterion's kind: %w", err)
	}
	return kind, nil
}

// evidenceIDFor answers the id of the row already recorded for this source.
func evidenceIDFor(ctx context.Context, tx pgx.Tx, in EvidenceInput) (ids.UUID, error) {
	var id ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT id FROM deal_stage_evidence
		 WHERE deal_id = $1 AND criterion_id = $2 AND source_type = $3 AND source_id = $4`,
		in.DealID, in.CriterionID, in.SourceType, in.SourceID).Scan(&id)
	if err != nil {
		return id, fmt.Errorf("read the standing evidence for this source: %w", err)
	}
	return id, nil
}

// readEvidence answers one row by id.
func readEvidence(ctx context.Context, tx pgx.Tx, id ids.UUID) (crmcontracts.StageEvidence, error) {
	e, err := scanEvidence(tx.QueryRow(ctx,
		`SELECT `+evidenceColumns+` FROM deal_stage_evidence WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return crmcontracts.StageEvidence{}, apperrors.ErrNotFound
	}
	return e, err
}

// readDealEvidence answers a deal's ledger, newest observation first.
func readDealEvidence(
	ctx context.Context, tx pgx.Tx, dealID ids.DealID,
) ([]crmcontracts.StageEvidence, error) {
	rows, err := tx.Query(ctx, `SELECT `+evidenceColumns+`
		  FROM deal_stage_evidence WHERE deal_id = $1
		 ORDER BY observed_at DESC, id DESC`, dealID)
	if err != nil {
		return nil, fmt.Errorf("list stage evidence: %w", err)
	}
	defer rows.Close()
	out := []crmcontracts.StageEvidence{}
	for rows.Next() {
		e, err := scanEvidence(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read stage evidence: %w", err)
	}
	return out, nil
}

func scanEvidence(row scannable) (crmcontracts.StageEvidence, error) {
	var out crmcontracts.StageEvidence
	var id, dealID, criterionID, sourceID ids.UUID
	var refutedBy *ids.UUID
	var sourceType, authorSide, commitment string
	var version int64
	if err := row.Scan(&id, &dealID, &criterionID, &sourceType, &sourceID,
		&out.SourceLines, &out.Snippet, &authorSide, &commitment, &out.Met,
		&out.Confidence, &out.ObservedAt, &out.ExtractedBy, &out.RefutedAt,
		&refutedBy, &out.CreatedAt, &out.UpdatedAt, &version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, err
		}
		return out, fmt.Errorf("scan stage evidence: %w", err)
	}
	out.Id = openapi_types.UUID(id)
	out.DealId = openapi_types.UUID(dealID)
	out.CriterionId = openapi_types.UUID(criterionID)
	out.SourceId = openapi_types.UUID(sourceID)
	out.SourceType = crmcontracts.StageEvidenceSource(sourceType)
	out.AuthorSide = crmcontracts.StageEvidenceAuthorSide(authorSide)
	out.Commitment = crmcontracts.StageEvidenceCommitment(commitment)
	if refutedBy != nil {
		id := openapi_types.UUID(*refutedBy)
		out.RefutedBy = &id
	}
	out.Version = &version
	return out, nil
}

// evidenceAfter is the audit after-image of one observation. author_side and
// met are the two a later reader judges the claim by, so both are named.
func evidenceAfter(e crmcontracts.StageEvidence) map[string]any {
	return map[string]any{
		"deal_id": e.DealId.String(), "criterion_id": e.CriterionId.String(),
		"source_type": string(e.SourceType), "source_id": e.SourceId.String(),
		"author_side": string(e.AuthorSide), "commitment": string(e.Commitment),
		"met": e.Met, "extracted_by": e.ExtractedBy,
	}
}

// announceEvidence writes the create's audit row and the deal event.
func announceEvidence(ctx context.Context, tx pgx.Tx,
	dealID ids.DealID, id ids.UUID, after map[string]any,
) error {
	auditID, err := storekit.AuditEvent(ctx, tx, "create", evidenceEntity, id, after)
	if err != nil {
		return fmt.Errorf("audit stage evidence create: %w", err)
	}
	return emitEvidenceChanged(ctx, tx, auditID, dealID)
}

// emitEvidenceChanged publishes the one fact an evidence write makes: what is
// known about this deal's criteria changed.
//
// It rides deal.updated because evidence is a fact ABOUT the deal, and the
// payload names no evidence field: a subscriber that needs the ledger re-reads
// it, and putting a snippet on the bus would publish quoted buyer text to
// every consumer of a deal rename.
func emitEvidenceChanged(
	ctx context.Context, tx pgx.Tx, auditID ids.UUID, dealID ids.DealID,
) error {
	if err := storekit.EmitEvent(ctx, tx, auditID, dealID.UUID,
		crmcontracts.PublicEventDealUpdated{
			ChangedFields: map[string]any{"stage_evidence": true},
		}); err != nil {
		return fmt.Errorf("emit deal.updated for its stage evidence: %w", err)
	}
	return nil
}
