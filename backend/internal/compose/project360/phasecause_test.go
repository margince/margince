// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package project360

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAManualMoveCarriesNoCause(t *testing.T) {
	if got := phaseCause(nil, map[ids.UUID]string{}); got != nil {
		t.Errorf("cause = %+v, want nil for a move a person made", got)
	}
}

func TestAWonDealCauseCarriesTheDealTheReaderCanSee(t *testing.T) {
	deal := ids.NewV7()
	got := phaseCause(&projects.PhaseCause{Kind: projects.PhaseCauseDealWon, DealID: deal},
		map[ids.UUID]string{deal: "ERP rollout"})
	if got == nil || got.Kind != crmcontracts.Project360PhaseCauseKindDealWon {
		t.Fatalf("cause = %+v, want deal_won", got)
	}
	if got.DealId == nil || ids.UUID(*got.DealId) != deal || got.DealName == nil || *got.DealName != "ERP rollout" {
		t.Errorf("cause names %v %v, want the deal's id and name", got.DealId, got.DealName)
	}
}

func TestAWonDealCauseLeavesAnUnseenDealUnnamed(t *testing.T) {
	got := phaseCause(&projects.PhaseCause{Kind: projects.PhaseCauseDealWon, DealID: ids.NewV7()},
		map[ids.UUID]string{})
	if got == nil || got.Kind != crmcontracts.Project360PhaseCauseKindDealWon {
		t.Fatalf("cause = %+v, want deal_won kept", got)
	}
	if got.DealId != nil || got.DealName != nil {
		t.Errorf("cause names %v %v, want both null", got.DealId, got.DealName)
	}
}
