// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The stage-automation report over its real HTTP surface.
//
// The store's own tests hold the arithmetic. What this holds is that the
// endpoint answers at all, with the shape the frontend reads and the rates
// computed server-side — a report whose numbers were right in Go and absent on
// the wire would look, from the settings page, exactly like a feature nobody
// built.

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/installseam"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheStageAutomationReportAnswersOverHTTP(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	pipeline, open, _ := DealFixture(t, e)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Report subject", pipeline, open, &e.Rep1))

	// A settled proposal, written through the real ledger writers rather than
	// inserted: a hand-built row would prove the SQL reads a shape the product
	// does not write.
	if _, err := e.Deals.CreateStageExitCriterion(admin, deals.CreateCriterionInput{
		StageID: open, Key: "signed", Label: "The agreement is signed",
		Kind: string(deals.CriterionDocumentSigned),
	}); err != nil {
		t.Fatalf("configuring the criterion: %v", err)
	}
	if _, err := e.Deals.RecordDeterministicEvidence(admin, deals.DeterministicClaim{
		DealID: deal, Kind: deals.CriterionDocumentSigned, SourceType: "contract",
		SourceID: ids.NewV7(), AuthorSide: deals.AuthorBuyer,
		ObservedAt: stageProgressionClock.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("recording the evidence: %v", err)
	}
	proposer := compose.NewStageProgressionProposer(e.Pool, e.Deals,
		approvals.NewService(e.DB()),
		func() time.Time { return stageProgressionClock },
		slog.New(slog.DiscardHandler))
	if staged, err := proposer.Propose(admin, deal); err != nil || !staged {
		t.Fatalf("proposing the move: staged=%v err=%v", staged, err)
	}
	var approvalID ids.UUID
	if err := e.Pool.QueryRow(t.Context(), `
		SELECT id FROM approval
		 WHERE kind = $1 AND target_entity_id = $2 AND status = 'pending'`,
		deals.StageProgressionKind, deal).Scan(&approvalID); err != nil {
		t.Fatalf("reading the staged card: %v", err)
	}
	if err := e.Deals.RecordProgressionDecided(
		admin, approvalID, deals.ProgressionApprovedClean, nil, false); err != nil {
		t.Fatalf("recording the acceptance: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/stage-automation/report", nil).
		WithContext(admin)
	deals.NewHandlers(e.DB(), installseam.Deals()).GetStageAutomationReport(rec, req,
		crmcontracts.GetStageAutomationReportParams{PipelineId: openapi_types.UUID(pipeline.UUID)})
	if rec.Code != http.StatusOK {
		t.Fatalf("report: status %d, body %s", rec.Code, rec.Body.String())
	}

	var got crmcontracts.StageAutomationReport
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode the report: %v", err)
	}
	if got.WindowDays != 30 {
		t.Errorf("the report says it covers %d days, want the 30 the gate reads",
			got.WindowDays)
	}
	if len(got.Data) != 1 {
		t.Fatalf("the report carries %d transitions, want 1", len(got.Data))
	}
	row := got.Data[0]
	if row.Reviewed != 1 || row.AcceptedClean != 1 {
		t.Fatalf("the accepted move reads %d clean of %d reviewed, want 1 of 1",
			row.AcceptedClean, row.Reviewed)
	}
	// The RATE, computed server-side. A client dividing the counts itself would
	// be writing a second definition of clean acceptance.
	if row.CleanAcceptanceRate != 1 {
		t.Errorf("clean acceptance reads %.2f on the wire, want 1", row.CleanAcceptanceRate)
	}
	if row.FromStageName == "" || row.ToStageName == "" {
		t.Error("the transition is unnamed on the wire, so the settings table can only " +
			"print two uuids at a reader deciding whether to trust it")
	}
}

// An unknown pipeline is NOT an empty report.
//
// The two look identical on screen — no rows — and mean opposite things: one
// says nothing has been proposed yet, the other says the id is wrong. A reader
// shown "nothing proposed" for a pipeline that does not exist would wait for
// numbers that can never arrive.
func TestAnUnknownPipelineIsNotAnEmptyReport(t *testing.T) {
	e := Setup(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/stage-automation/report", nil).
		WithContext(e.Admin())
	deals.NewHandlers(e.DB(), installseam.Deals()).GetStageAutomationReport(rec, req,
		crmcontracts.GetStageAutomationReportParams{PipelineId: openapi_types.UUID(ids.NewV7())})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("an unknown pipeline answered %d, want 404 — an empty report would tell "+
			"the reader to wait for numbers that can never arrive", rec.Code)
	}
}
