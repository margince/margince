// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestImportApprovalPinsReviewedChoicesAcrossNativeDraftEdits(t *testing.T) {
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
	if _, err := service.Decide(ctx, id, true, nil); err != nil {
		t.Fatal(err)
	}
	changedKey := "Name"
	changed, err := runs.UpdateDraft(ctx, run.ID, run.Version, migration.DraftPatch{SourceKey: &changedKey})
	if err != nil {
		t.Fatal(err)
	}
	if changed.Version != run.Version+1 || changed.Report != nil || changed.Status != migration.StatusPending {
		t.Fatalf("changed choices retained the reviewed revision: %+v", changed)
	}
	if _, _, _, err := agents.RedeemAndMark(ctx, approvalsAdapter{svc: service}, id, "commit_import", hash); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("redemption after changing reviewed choices = %v, want version skew", err)
	}
}
