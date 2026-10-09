// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The quotation a deal's stage evidence holds of a message, when that message's
// text is destroyed.
//
// Both destructive engines empty the message in place, and the evidence names
// its source polymorphically, so no foreign key carries the destruction over.
// Each engine is asked through its own entry point. The owner purge runs the
// per-activity arm the retention sweep and a restriction release share.

import (
	"log/slog"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const quotedAcceptance = "Mara Kessler: The terms work for us, please send the order form."

// evidenceQuoting records one model claim on a fresh deal, through the real
// ledger writer, quoting the given line of the given activity.
func evidenceQuoting(t *testing.T, e *integration.Env, activityID ids.UUID, quote string) ids.UUID {
	t.Helper()
	ctx := e.Admin()
	pipeline, err := e.Deals.CreatePipeline(ctx, deals.CreatePipelineInput{Name: "Sales " + ids.NewV7().String()})
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
		SourceType:  deals.SourceActivity,
		SourceID:    activityID,
		SourceLines: []int32{2},
		Snippet:     &quote,
		AuthorSide:  deals.AuthorBuyer,
		Commitment:  deals.CommitmentAgreed,
		Met:         true,
		Confidence:  &confidence,
		ObservedAt:  time.Now().UTC(),
		ExtractedBy: stageEvidenceExtractedBy,
	})
	if err != nil {
		t.Fatalf("recording the evidence: %v", err)
	}
	return ids.UUID(written.Id)
}

// quotingRows counts the evidence rows that still carry the words.
func quotingRows(t *testing.T, e *integration.Env, words string) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM deal_stage_evidence WHERE snippet = $1`, words)
}

// snippetCleared answers whether the evidence row stands with no quotation at all.
func snippetCleared(t *testing.T, e *integration.Env, evidence ids.UUID) bool {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM deal_stage_evidence WHERE id = $1 AND snippet IS NULL`, evidence) == 1
}

func TestErasureClearsTheStageEvidenceQuotingAnErasedMessage(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Mara Kessler", nil)
	erased := seedTranscript(t, e, "1: Tom: Where are we on terms?\n2: "+quotedAcceptance)
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, contact_id)
		VALUES ($1, 'contact', $2)`, erased, contact)
	evidence := evidenceQuoting(t, e, erased, quotedAcceptance)

	// The same words from a meeting the erasure does not reach: the redaction
	// is keyed on the erased source, never on matching text.
	const keptQuote = "Bob Ferrer: The terms work for us, please send the order form."
	kept := seedTranscript(t, e, "1: Tom: Terms?\n2: "+keptQuote)
	evidenceQuoting(t, e, kept, keptQuote)

	if err := privacy.NewEraser(e.DB()).EraseContact(e.Admin(), contact, "subject request"); err != nil {
		t.Fatalf("EraseContact → %v", err)
	}

	if !snippetCleared(t, e, evidence) {
		t.Error("the evidence row still carries a quotation of the erased message, or went with it — the ledger keeps the claim and drops the words")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log
		WHERE entity_type = 'deal_stage_evidence' AND entity_id = $1 AND action = 'erase'`, evidence); n != 1 {
		t.Error("clearing the quotation left no erase tombstone on the evidence row")
	}
	if n := quotingRows(t, e, keptQuote); n != 1 {
		t.Error("the erasure cleared evidence quoting a message it did not erase")
	}
}

func TestAnOwnerPurgeClearsTheStageEvidenceQuotingThePurgedMessage(t *testing.T) {
	e := integration.Setup(t)
	purged := seedTranscript(t, e, "1: Tom: Where are we on terms?\n2: "+quotedAcceptance)
	evidence := evidenceQuoting(t, e, purged, quotedAcceptance)

	retention := NewRetentionServiceFor(InstallationDB(e.Pool), nil, slog.Default())
	if _, err := retention.PurgeActivities(e.Admin(), []ids.UUID{purged}, privacy.PurgeOwnerRule); err != nil {
		t.Fatalf("PurgeActivities → %v", err)
	}

	if !snippetCleared(t, e, evidence) {
		t.Error("stage evidence still quotes the purged message — its text is gone from the timeline and readable on the deal")
	}
}

// A model reading runs outside any transaction, so an erasure can commit while
// the call is out. The claim written after it must not bring the words back.
func TestAQuotationWrittenAfterTheErasureIsNotStored(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Mara Kessler", nil)
	erased := seedTranscript(t, e, "1: Tom: Where are we on terms?\n2: "+quotedAcceptance)
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, contact_id)
		VALUES ($1, 'contact', $2)`, erased, contact)
	if err := privacy.NewEraser(e.DB()).EraseContact(e.Admin(), contact, "subject request"); err != nil {
		t.Fatalf("EraseContact → %v", err)
	}

	evidence := evidenceQuoting(t, e, erased, quotedAcceptance)

	if !snippetCleared(t, e, evidence) {
		t.Error("a reading that finished after the erasure stored its quotation of the erased message")
	}
}

// What the ledger read out of the subject's messages is a conclusion about
// their words, and the access export hands it back.
func TestTheAccessExportCarriesTheStageEvidenceReadFromTheSubjectsMessages(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Mara Kessler", nil)
	activity := seedTranscript(t, e, "1: Tom: Where are we on terms?\n2: "+quotedAcceptance)
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, contact_id)
		VALUES ($1, 'contact', $2)`, activity, contact)
	evidenceQuoting(t, e, activity, quotedAcceptance)

	pkg, err := privacy.AssembleSAR(e.Admin(), e.DB(), ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatalf("AssembleSAR → %v", err)
	}
	if len(pkg.StageEvidence) != 1 || pkg.StageEvidence[0]["snippet"] != quotedAcceptance ||
		pkg.StageEvidence[0]["criterion"] != "Terms accepted" {
		t.Errorf("the export does not carry the claim read from the subject's message: %+v", pkg.StageEvidence)
	}
}
