// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestReviewedImportReplaysItsOwnAtomicUpdateButRefusesAnInterveningEdit(t *testing.T) {
	e := integration.Setup(t)
	handler, initial := approvedLeadRun(t, e)
	if err := commitWorkerFor(e, handler).Work(context.Background(), &river.Job[ImportCommitArgs]{Args: initial}); err != nil {
		t.Fatal(err)
	}
	ctx := importAdmin(e)
	profile, err := handler.profileAndStore(ctx, migration.ObjectLead, "corrected-leads.csv", []byte("Email,Full Name\nada@lovelace.example,Ada Lovelace\n"))
	if err != nil {
		t.Fatal(err)
	}
	validation := queuedLeadTest(t, e, handler, profile)
	if err := validateWorkerFor(e, handler).Work(context.Background(), &river.Job[ImportValidateArgs]{Args: validation}); err != nil {
		t.Fatal(err)
	}
	run, err := migration.NewRunStore(e.DB()).GetStaged(ctx, validation.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Report == nil || run.Report.ReviewPlan == nil || len(run.Report.ReviewPlan.Rows) != 1 || run.Report.ReviewPlan.Rows[0].Outcome != "update" {
		t.Fatalf("native test did not prepare the expected update: %+v", run.Report)
	}
	run.Report.ReviewPlan.Enforced = true
	source, err := importSourceFromReview(*run.Report.ReviewPlan, *run.Mapping)
	if err != nil {
		t.Fatal(err)
	}
	row := source.rows[migration.ObjectLead][0]
	batch := storekit.WithImportRun(storekit.WithBatch(ctx, run.ID), run.ID)
	land := func() (migration.EnsureResult, error) {
		writers, err := newBundleWriters(batch, e.DB(), run.ID, run.Mapping, source)
		if err != nil {
			return migration.EnsureResult{}, err
		}
		if err := writers.bindReviewPlan(run.Report); err != nil {
			return migration.EnsureResult{}, err
		}
		return writers.Ensure(batch, migration.ObjectLead, row)
	}
	first, err := land()
	if err != nil || first.Unchanged || first.Skipped {
		t.Fatalf("reviewed update did not land: %+v / %v", first, err)
	}
	replayed, err := land()
	if err != nil || !replayed.Unchanged {
		t.Fatalf("lost-checkpoint replay did not recognize its own native write: %+v / %v", replayed, err)
	}
	target, err := ids.Parse(run.Report.ReviewPlan.Rows[0].TargetID)
	if err != nil {
		t.Fatal(err)
	}
	store := contacts.NewStore(e.DB())
	if _, err := store.UpdateLead(ctx, ids.From[ids.LeadKind](target), contacts.UpdateLeadInput{Title: new("Changed by a colleague")}); err != nil {
		t.Fatal(err)
	}
	if _, err := land(); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("an unrelated edit was mistaken for this import's completed write: %v", err)
	}
}
