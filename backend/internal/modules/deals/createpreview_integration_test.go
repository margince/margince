// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestDealCreationPreviewMatchesWriterWithoutWriting(t *testing.T) {
	e := setupConfigEnv(t)
	e.store.installation = eurInstallation()
	stage := openStage(t, e, SemanticOpen)
	var pipeline ids.PipelineID
	if err := e.owner.QueryRow(t.Context(), `SELECT pipeline_id FROM stage WHERE id=$1`, stage).Scan(&pipeline); err != nil {
		t.Fatal(err)
	}
	creatorCtx := e.asDealWriter()
	creator, ok := principal.Actor(creatorCtx)
	if !ok {
		t.Fatal("missing fixture creator")
	}
	reviewerCtx := principal.WithActor(creatorCtx, principal.Principal{Type: principal.PrincipalSystem, ID: "system:approval-preview"})
	amount, currency := int64(125000), "EUR"
	input := CreateDealInput{Name: "New opportunity", PipelineID: pipeline, StageID: stage, AmountMinor: &amount, Currency: &currency, Source: "manual"}
	preview, err := e.store.PreviewDealCreationForActor(reviewerCtx, input, creator)
	if err != nil {
		t.Fatal(err)
	}
	if preview.OwnerID == nil || preview.OwnerID.UUID != e.admin {
		t.Fatalf("creator owner lost: %+v", preview.OwnerID)
	}
	var count int
	if err := e.owner.QueryRow(t.Context(), `SELECT count(*) FROM deal WHERE pipeline_id=$1`, pipeline).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("preview wrote %d deals", count)
	}
	actual, err := e.store.CreateDeal(creatorCtx, input)
	if err != nil {
		t.Fatal(err)
	}
	if actual.OwnerId == nil || ids.UUID(*actual.OwnerId) != preview.OwnerID.UUID || actual.Name != preview.Name || actual.AmountMinor == nil || *actual.AmountMinor != *preview.AmountMinor || actual.Currency == nil || *actual.Currency != *preview.Currency {
		t.Fatalf("writer differs from preview: %+v / %+v", actual, preview)
	}
}

func TestDealCreationPreviewDoesNotBorrowReviewerCreateGrant(t *testing.T) {
	e := setupConfigEnv(t)
	creator, ok := principal.Actor(e.asDealless())
	if !ok {
		t.Fatal("missing fixture creator")
	}
	_, err := e.store.PreviewDealCreationForActor(e.asDealWriter(), CreateDealInput{}, creator)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("creator without grant admitted: %v", err)
	}
	reviewer, ok := principal.Actor(e.asDealWriter())
	if !ok {
		t.Fatal("missing fixture reviewer")
	}
	_, err = e.store.PreviewDealCreationForActor(e.asDealless(), CreateDealInput{}, reviewer)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("reviewer without grant admitted: %v", err)
	}
}
