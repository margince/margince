// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A reviewed plan lands each row inside one shared transaction. A row the
// store refuses as a duplicate is skipped without aborting that transaction.

import (
	"context"
	"testing"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

func TestAReviewedLeadRowNamingAHeldEmailIsSkippedAndTheNextRowLands(t *testing.T) {
	e := integration.Setup(t)
	e.GrantRole(t, e.AdminUser, "admin")
	h, _ := leadSourceHandlers(t, e)
	ctx := importAdmin(e)
	object := crmcontracts.ImportObject(migration.ObjectLead)
	mapping := map[string]string{"Ref": "ref", "Email": "email", "Full Name": "full_name"}
	key := "Ref"
	stage := func(file string) migration.Run {
		profile, err := h.profileAndStore(ctx, migration.ObjectLead, "leads.csv", []byte(file))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.stageRun(ctx, crmcontracts.CreateImportRunRequest{
			Connector: crmcontracts.CreateImportRunRequestConnectorCsv, Object: &object,
			SourceRef: profile.SourceRef, Mapping: &mapping, SourceKey: &key,
		}); err != nil {
			t.Fatal(err)
		}
		run, err := migration.NewRunStore(e.DB()).GetStaged(ctx, migration.RunID(profile.RunId))
		if err != nil {
			t.Fatal(err)
		}
		args := ImportValidateArgs{Workspace: e.WS, RunID: run.ID, Attempt: run.JobAttempt}
		if err := validateWorkerFor(e, h).Work(context.Background(), &river.Job[ImportValidateArgs]{Args: args}); err != nil {
			t.Fatal(err)
		}
		run, err = migration.NewRunStore(e.DB()).GetStaged(ctx, run.ID)
		if err != nil {
			t.Fatal(err)
		}
		return run
	}

	first := stage("Ref,Email,Full Name\nr1,a@lead.example,Ada\n")
	approved, err := migration.NewRunStore(e.DB()).Approve(ctx, first.ID, 0, workedByHand)
	if err != nil {
		t.Fatal(err)
	}
	args := ImportCommitArgs{Workspace: e.WS, RunID: approved.ID, Attempt: approved.JobAttempt}
	if err := commitWorkerFor(e, h).Work(context.Background(), &river.Job[ImportCommitArgs]{Args: args}); err != nil {
		t.Fatal(err)
	}
	store := contacts.NewStore(e.DB())
	if _, _, err := store.CreateLead(ctx, contacts.CreateLeadInput{FullName: new("Made In App"), Email: new("b@lead.example"), Status: "new"}); err != nil {
		t.Fatal(err)
	}

	run := stage("Ref,Email,Full Name\nr1,b@lead.example,Ada\nr2,c@lead.example,Cy\nr3,b@lead.example,Dee\n")
	if run.Report == nil || run.Report.ReviewPlan == nil || len(run.Report.ReviewPlan.Rows) != 3 {
		t.Fatalf("the test run prepared %+v, want a plan of the three rows", run.Report)
	}
	run.Report.ReviewPlan.Enforced = true
	source, err := importSourceFromReview(*run.Report.ReviewPlan, *run.Mapping)
	if err != nil {
		t.Fatal(err)
	}
	batch := storekit.WithImportRun(storekit.WithBatch(ctx, run.ID), run.ID)
	for _, row := range source.rows[migration.ObjectLead] {
		writers, err := newBundleWriters(batch, e.DB(), run.ID, run.Mapping, source)
		if err != nil {
			t.Fatal(err)
		}
		if err := writers.bindReviewPlan(run.Report); err != nil {
			t.Fatal(err)
		}
		landed, err := writers.Ensure(batch, migration.ObjectLead, row)
		if err != nil {
			t.Fatalf("row %s: %v, want the row skipped without failing the run", row.ExternalID, err)
		}
		if skipped := row.ExternalID != "r2"; landed.Skipped != skipped {
			t.Fatalf("row %s landed %+v, want skipped=%v", row.ExternalID, landed, skipped)
		}
	}
}
