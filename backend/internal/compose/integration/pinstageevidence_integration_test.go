// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A controller pinning a record to its statutory floor restricts it. The body
// is kept for the obligation and reads as gone, so the deal stops quoting it.
// Here rather than beside the other evidence cases because a pin needs the
// floor this package arms.
func TestPinningAMessageClearsTheStageEvidenceQuotingIt(t *testing.T) {
	e := Setup(t)
	const quote = "The terms work for us, please send the order form."
	pinned := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, body, counterparty_email, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'Order', $2, 'supplier@parts.test', now() - interval '30 days', 'manual', 'human:x')`,
		pinned, quote)
	evidence := recordModelEvidence(t, e, pinned, quote)

	reason, err := privacy.ParseStatedReason("supplier correspondence: §257 HGB, no deal in the CRM")
	if err != nil {
		t.Fatal(err)
	}
	controller := controllerCtx(e, principal.ObjectGrant{Read: true, Update: true})
	if err := privacy.NewEraser(e.DB()).PinToFloor(controller, pinned, reason); err != nil {
		t.Fatalf("PinToFloor → %v", err)
	}

	if n := e.WsCount(t, `SELECT count(*) FROM deal_stage_evidence WHERE id = $1 AND snippet IS NULL`, evidence); n != 1 {
		t.Error("a deal still quotes a message that is held and reads as gone to every reader")
	}
}

// recordModelEvidence writes one model claim quoting the activity on a fresh
// deal, through the real ledger writer.
func recordModelEvidence(t *testing.T, e *Env, activityID ids.UUID, quote string) ids.UUID {
	t.Helper()
	ctx := e.Admin()
	pipeline, err := e.Deals.CreatePipeline(ctx, deals.CreatePipelineInput{Name: "Sales"})
	if err != nil {
		t.Fatalf("seeding a pipeline: %v", err)
	}
	pipelineID := ids.From[ids.PipelineKind](ids.UUID(pipeline.Id))
	stage, err := e.Deals.CreateStage(ctx, deals.CreateStageInput{
		PipelineID: pipelineID, Name: "Negotiation", Position: 0, Semantic: string(deals.SemanticOpen),
	})
	if err != nil {
		t.Fatalf("seeding a stage: %v", err)
	}
	stageID := ids.From[ids.StageKind](ids.UUID(stage.Id))
	criterion, err := e.Deals.CreateStageExitCriterion(ctx, deals.CreateCriterionInput{
		StageID: stageID, Key: "terms", Label: "Terms accepted", Kind: string(deals.CriterionTermsAccepted),
	})
	if err != nil {
		t.Fatalf("seeding the criterion: %v", err)
	}
	dealID := e.SeedDeal(t, "Warehouse rollout", pipelineID, stageID, &e.AdminUser)
	confidence := 0.9
	written, err := e.Deals.RecordStageEvidence(ctx, deals.EvidenceInput{
		DealID:      ids.From[ids.DealKind](dealID),
		CriterionID: ids.From[ids.ExitCriterionKind](ids.UUID(criterion.Id)),
		SourceType:  deals.SourceActivity, SourceID: activityID, Snippet: &quote,
		AuthorSide: deals.AuthorBuyer, Commitment: deals.CommitmentAgreed, Met: true,
		Confidence: &confidence, ObservedAt: time.Now().UTC(), ExtractedBy: "stage_evidence_extract",
	})
	if err != nil {
		t.Fatalf("recording the evidence: %v", err)
	}
	return ids.UUID(written.Id)
}
