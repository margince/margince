// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestImportApprovalPinsReviewedChoicesAcrossNativeDraftEdits(t *testing.T) {
	e := integration.Setup(t)
	e.GrantRole(t, e.AdminUser, "admin")
	ctx := importAdmin(e)
	handler, _ := leadSourceHandlers(t, e)
	run := testedImportRun(t, e, handler, migration.ObjectLead, testedLeadFile, testedLeadMapping(), crmcontracts.ImportOnDuplicateCreate)
	runs := migration.NewRunStore(e.DB())
	service, id, hash := stagedImportApproval(t, e, run)
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
