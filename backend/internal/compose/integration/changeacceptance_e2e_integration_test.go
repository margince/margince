// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose"
)

func TestAnAutomaticStageMoveCanBeAcceptedWithoutMovingItAgain(t *testing.T) {
	e := Setup(t)
	deal, staged := proposeOnMetCriteria(t, e)
	if !staged {
		t.Fatal("no proposal staged")
	}
	ref := governedTransition(t, e, deal)
	count, err := compose.SweepAutoApply(e.Admin(), e.Pool)
	if err != nil || count == 0 {
		t.Fatalf("automatic stage move: %d, %v", count, err)
	}
	card := appliedCardFor(t, e, deal)
	deliverApprovalDecided(t, e, card)
	review, err := e.Deals.AppliedChangeReview(e.Admin(), deal, card.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if review.Kind != "stage" || !review.CanUndo || !review.CanAccept {
		t.Fatalf("automatic move has no review actions: %+v", review)
	}
	if err := e.Deals.AcceptAppliedChange(e.Admin(), deal, card.UUID, review.Version); err != nil {
		t.Fatal(err)
	}
	accepted, err := e.Deals.AppliedChangeReview(e.Admin(), deal, card.UUID)
	if err != nil || !accepted.Accepted {
		t.Fatalf("acceptance did not persist: %+v, %v", accepted, err)
	}
	if stageOf(t, e, deal) != ref.ToStageID {
		t.Fatal("acceptance moved the deal again")
	}
}
