// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// Only a revalidation writes; every other category is kept and named. An
// item the classifier could not place is a conflict, never a quiet keep.
func TestEveryLegacySettlementCategoryHasOneAction(t *testing.T) {
	for category, want := range map[activities.SettlementRepairCategory]homeRepairAction{
		activities.SettlementRevalidate:        repairRevalidateSettlement,
		activities.SettlementProvenanceUnknown: repairHumanReview,
		activities.SettlementHumanWork:         repairLeaveCurrent,
		activities.SettlementPastHorizon:       repairLeaveCurrent,
		activities.SettlementNoAnswers:         repairHumanReview,
		activities.SettlementPending:           repairLeaveCurrent,
		activities.SettlementUnavailable:       repairHumanReview,
		activities.SettlementChanged:           repairLeaveCurrent,
	} {
		ref, err := homeRepairSettlementRef(activities.SettlementRepairItem{Category: category})
		if err != nil || ref.Action != want || ref.Source != repairSourceSettlement {
			t.Errorf("%s = %+v (%v), want %s", category, ref, err, want)
		}
	}
	if _, err := homeRepairSettlementRef(activities.SettlementRepairItem{}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("an unplaced verdict = %v, want a conflict", err)
	}
}
