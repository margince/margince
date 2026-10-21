// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestImportApprovalReleasesTheDisplayedRowsThroughTheQueuedWriter(t *testing.T) {
	e := integration.Setup(t)
	e.GrantRole(t, e.AdminUser, "admin")
	ctx := importAdmin(e)
	handler, source := leadSourceHandlers(t, e)
	validation := queuedLeadTest(t, e, handler, source)
	if err := validateWorkerFor(e, handler).Work(context.Background(), &river.Job[ImportValidateArgs]{Args: validation}); err != nil {
		t.Fatal(err)
	}
	runs := migration.NewRunStore(e.DB())
	run, err := runs.GetStaged(ctx, validation.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Report == nil || run.Report.ReviewPlan == nil || !run.Report.ReviewPlan.Complete {
		t.Fatalf("native validation did not retain a complete row plan: %+v", run.Report)
	}
	raw, err := json.Marshal(map[string]ids.UUID{"run_id": ids.UUID(run.ID)})
	if err != nil {
		t.Fatal(err)
	}
	canonical, hash, err := diffhash.Canonical(raw)
	if err != nil {
		t.Fatal(err)
	}
	service := NewApprovalsService(e.DB())
	id, err := service.Stage(ctx, approvals.StageInput{Kind: "commit_import", TargetType: "import_run", TargetID: ids.UUID(run.ID), ProposedChange: canonical, DiffHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := service.GetWire(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Preview == nil || proposal.Preview.State != "ready" || proposal.Preview.ImportRows == nil || len(*proposal.Preview.ImportRows) != 1 {
		t.Fatalf("complete native row plan is not decidable: preview=%+v target_version=%v run=%+v plan=%+v", proposal.Preview, proposal.TargetVersion, run, run.Report.ReviewPlan)
	}
	row := (*proposal.Preview.ImportRows)[0]
	if row.Object != migration.ObjectLead || row.Outcome != "create" || row.SourceKey != "ada@lovelace.example" {
		t.Fatalf("wrong row promised: %+v", row)
	}
	foundName, foundEmail := false, false
	for _, detail := range row.Details {
		foundName = foundName || detail.Field == "full_name" && detail.Value.Text == "Ada"
		foundEmail = foundEmail || detail.Field == "email" && detail.Value.Text == "ada@lovelace.example"
	}
	if !foundName || !foundEmail {
		t.Fatalf("native values missing from decision: %+v", row.Details)
	}
	if _, err := service.Decide(ctx, id, true, nil); err != nil {
		t.Fatal(err)
	}
	released, _, _, err := agents.RedeemAndMark(ctx, approvalsAdapter{svc: service}, id, "commit_import", hash)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := runs.Approve(released, run.ID, 0, workedByHand)
	if err != nil {
		t.Fatal(err)
	}
	if approved.Report == nil || approved.Report.ReviewPlan == nil || !approved.Report.ReviewPlan.Enforced {
		t.Fatal("redemption did not preserve row guards for the worker")
	}
	args := ImportCommitArgs{Workspace: e.WS, RunID: run.ID, Attempt: approved.JobAttempt}
	worker := commitWorkerFor(e, handler)
	if err := worker.Work(context.Background(), &river.Job[ImportCommitArgs]{Args: args}); err != nil {
		t.Fatal(err)
	}
	finished, err := runs.GetStaged(ctx, run.ID)
	if err != nil || finished.Status != migration.StatusComplete || finished.Report == nil || finished.Report.Imported != 1 {
		t.Fatalf("approved row did not land: %+v / %v", finished, err)
	}
	if !finished.SourceReleased || finished.Report.ReviewPlan != nil {
		t.Fatal("completed import retained source cells in its review plan")
	}
	if err := worker.Work(context.Background(), &river.Job[ImportCommitArgs]{Args: args}); err != nil {
		t.Fatalf("completed delivery was not idempotent: %v", err)
	}
}
