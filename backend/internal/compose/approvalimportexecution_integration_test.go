// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const testedLeadFile = "Email,Full Name\nada@lovelace.example,Ada\n"

// testedLeadMapping places the columns of testedLeadFile.
func testedLeadMapping() map[string]string {
	return map[string]string{"Email": "email", "Full Name": "full_name"}
}

// testedImportRun stores a file, starts its test run as the handler does, and
// works the test run's job.
func testedImportRun(t *testing.T, e *integration.Env, handler importHandlers, object, file string,
	mapping map[string]string, onDuplicate crmcontracts.ImportOnDuplicate,
) migration.Run {
	t.Helper()
	ctx := importAdmin(e)
	profile, err := handler.profileAndStore(ctx, object, object+"s.csv", []byte(file))
	if err != nil {
		t.Fatal(err)
	}
	return testedRun(t, e, handler, profile, crmcontracts.CreateImportRunRequest{
		Object: new(crmcontracts.ImportObject(object)), Mapping: &mapping, OnDuplicate: &onDuplicate,
	})
}

// testedRun starts the test run req asks for on a stored source, and works its job.
func testedRun(t *testing.T, e *integration.Env, handler importHandlers, profile crmcontracts.ImportSourceProfile,
	req crmcontracts.CreateImportRunRequest,
) migration.Run {
	t.Helper()
	ctx := importAdmin(e)
	req.Connector, req.SourceRef = crmcontracts.CreateImportRunRequestConnectorCsv, profile.SourceRef
	if _, err := handler.stageRun(ctx, req); err != nil {
		t.Fatal(err)
	}
	runs := migration.NewRunStore(e.DB())
	run, err := runs.GetStaged(ctx, migration.RunID(profile.RunId))
	if err != nil {
		t.Fatal(err)
	}
	args := ImportValidateArgs{Workspace: e.WS, RunID: run.ID, Attempt: run.JobAttempt}
	if err := validateWorkerFor(e, handler).Work(context.Background(), &river.Job[ImportValidateArgs]{Args: args}); err != nil {
		t.Fatal(err)
	}
	if run, err = runs.GetStaged(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	return run
}

// stagedImportApproval stages the commit of a tested run for a decision.
func stagedImportApproval(t *testing.T, e *integration.Env, run migration.Run) (*approvals.Service, ids.ApprovalID, string) {
	t.Helper()
	canonical, hash, err := diffhash.Canonical(fmt.Appendf(nil, `{"run_id":%q}`, ids.UUID(run.ID)))
	if err != nil {
		t.Fatal(err)
	}
	service := NewApprovalsService(e.DB())
	id, err := service.Stage(importAdmin(e), approvals.StageInput{Kind: "commit_import", TargetType: "import_run", TargetID: ids.UUID(run.ID), ProposedChange: canonical, DiffHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	return service, id, hash
}

// approvedThroughDecision accepts the staged commit and approves the run on
// what the decision released.
func approvedThroughDecision(t *testing.T, e *integration.Env, service *approvals.Service, id ids.ApprovalID, hash string, run migration.Run) migration.Run {
	t.Helper()
	ctx := importAdmin(e)
	if _, err := service.Decide(ctx, id, true, nil); err != nil {
		t.Fatal(err)
	}
	released, _, _, err := agents.RedeemAndMark(ctx, approvalsAdapter{svc: service}, id, "commit_import", hash)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := migration.NewRunStore(e.DB()).Approve(released, run.ID, 0, workedByHand)
	if err != nil {
		t.Fatal(err)
	}
	return approved
}

func TestImportApprovalReleasesTheDisplayedRowsThroughTheQueuedWriter(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	e.GrantRole(t, e.AdminUser, "admin")
	ctx := importAdmin(e)
	handler, _ := leadSourceHandlers(t, e)
	run := testedImportRun(t, e, handler, migration.ObjectLead, testedLeadFile, testedLeadMapping(), crmcontracts.ImportOnDuplicateCreate)
	if run.Report == nil || run.Report.ReviewPlan == nil || !run.Report.ReviewPlan.Complete {
		t.Fatalf("native validation did not retain a complete row plan: %+v", run.Report)
	}
	service, id, hash := stagedImportApproval(t, e, run)
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
	approved := approvedThroughDecision(t, e, service, id, hash, run)
	if approved.Report == nil || approved.Report.ReviewPlan == nil || !approved.Report.ReviewPlan.Enforced {
		t.Fatal("redemption did not preserve row guards for the worker")
	}
	args := ImportCommitArgs{Workspace: e.WS, RunID: run.ID, Attempt: approved.JobAttempt}
	worker := commitWorkerFor(e, handler)
	if err := worker.Work(context.Background(), &river.Job[ImportCommitArgs]{Args: args}); err != nil {
		t.Fatal(err)
	}
	finished, err := migration.NewRunStore(e.DB()).GetStaged(ctx, run.ID)
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

// approvedAsShown stages the commit of a tested run, checks the approval shows
// the outcomes want, approves it and works the commit. It answers the run once
// the commit completes.
func approvedAsShown(t *testing.T, e *integration.Env, handler importHandlers, run migration.Run, want []string) migration.Run {
	t.Helper()
	service, id, hash := stagedImportApproval(t, e, run)
	proposal, err := service.GetWire(importAdmin(e), id)
	if err != nil || proposal.Preview == nil || proposal.Preview.ImportRows == nil {
		t.Fatalf("the approval shows no rows: %+v (%v)", proposal.Preview, err)
	}
	var shown []string
	for _, row := range *proposal.Preview.ImportRows {
		shown = append(shown, string(row.Outcome))
	}
	if !slices.Equal(shown, want) {
		t.Fatalf("the approval shows %v, want %v", shown, want)
	}
	commit := ImportCommitArgs{Workspace: e.WS, RunID: run.ID, Attempt: approvedThroughDecision(t, e, service, id, hash, run).JobAttempt}
	if err := commitWorkerFor(e, handler).Work(context.Background(), &river.Job[ImportCommitArgs]{Args: commit}); err != nil {
		t.Fatal(err)
	}
	finished, err := migration.NewRunStore(e.DB()).GetStaged(importAdmin(e), run.ID)
	if err != nil || finished.Status != migration.StatusComplete || finished.Report == nil {
		t.Fatalf("the approved run = %+v (%v), want it complete", finished, err)
	}
	return finished
}

// A file renaming a stored company and naming it and a new one twice is shown
// with each second spelling skipped. The approved import holds every row to that.
func TestAnApprovedImportSkipsTheLookAlikeItsApproverWasShown(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	e.GrantRole(t, e.AdminUser, "admin")
	handler, _ := leadSourceHandlers(t, e)
	stored, err := contacts.NewStore(e.DB()).CreateCompany(importAdmin(e), contacts.CreateCompanyInput{DisplayName: "Kestrel Data", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	file := fmt.Sprintf("Id,Company\n%s,Skipco Trading\n,Skipco Trading GmbH\n,Nordwind Logistik\n,Nordwind Logistik AG\n", stored.Id)
	run := testedImportRun(t, e, handler, migration.ObjectCompany, file, map[string]string{"Id": "id", "Company": "display_name"}, crmcontracts.ImportOnDuplicateSkip)
	approvedAsShown(t, e, handler, run, []string{"update", importRowSkip, importRowCreate, importRowSkip})
}

// A ZIP whose contacts name a company of the same upload by its ref lands as
// approved. The check before the commit caches nothing the commit then reads.
func TestAnApprovedZipLinksItsContactsToItsOwnCompanies(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	e.GrantRole(t, e.AdminUser, "admin")
	handler, _ := leadSourceHandlers(t, e)
	body, err := zipImportFiles([]agents.ImportFile{
		{Object: migration.ObjectCompany, CSV: "ref,name,domain\nk1,Twinco,twinco.example\n"},
		{Object: migration.ObjectContact, CSV: "ref,full_name,email,company_ref\nc1,Anna Schmidt,anna.schmidt@gmail.com,k1\nc2,Anna Schmitt,a.schmitt@twinco.example,\n"},
	})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := handler.profileBundle(importAdmin(e), importUpload{FileName: pastedBundleName, Body: body})
	if err != nil {
		t.Fatal(err)
	}
	run := testedRun(t, e, handler, profile, crmcontracts.CreateImportRunRequest{OnDuplicate: new(crmcontracts.ImportOnDuplicateSkip), Parts: &[]crmcontracts.ImportPartRequest{
		{File: "companies.csv", Object: migration.ObjectCompany, Mapping: map[string]string{"ref": "ref", "name": "display_name", "domain": "domain"}},
		{File: "contacts.csv", Object: migration.ObjectContact, Mapping: map[string]string{"ref": "ref", "full_name": "full_name", "email": "email", "company_ref": "company_ref"}},
	}})
	if finished := approvedAsShown(t, e, handler, run, []string{importRowCreate, importRowCreate, importRowSkip}); finished.Report.Imported != 2 {
		t.Fatalf("the approved run imported %d, want the company and one contact", finished.Report.Imported)
	}
}
