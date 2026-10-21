// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestProgressionChecklistRequiresEveryOriginalCriterionVersion(t *testing.T) {
	first, second := ids.NewV7(), ids.NewV7()
	current := []crmcontracts.StageExitCriterion{
		{Id: crmcontracts.Id(first), Key: "signed", Label: "Agreement signed", Required: true, Version: new(int64(3))},
		{Id: crmcontracts.Id(second), Key: "terms", Label: "Terms accepted", Required: true, Version: new(int64(2))},
	}
	proposed := []ProposedCriterion{
		{CriterionID: ids.From[ids.ExitCriterionKind](first), Key: "signed", Label: "Agreement signed", Required: true, Version: 3},
		{CriterionID: ids.From[ids.ExitCriterionKind](second), Key: "terms", Label: "Terms accepted", Required: true, Version: 2},
	}
	if err := matchProgressionCriteria(proposed, current); err != nil {
		t.Fatal(err)
	}
	duplicate := append([]ProposedCriterion(nil), proposed...)
	duplicate[1] = duplicate[0]
	if err := matchProgressionCriteria(duplicate, current); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("duplicate checklist: %v", err)
	}
	proposed[0].Version = 0
	if err := matchProgressionCriteria(proposed, current); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("unversioned legacy checklist: %v", err)
	}
}
