// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type countedRoster struct {
	members []TeamMember
	reads   *int
}

func (countedRoster) SharesLiveTeamWithCaller(context.Context, ids.UUID) (bool, error) {
	return true, nil
}

func (r countedRoster) LiveTeammatesOfCaller(context.Context) ([]TeamMember, bool, error) {
	*r.reads++
	return r.members, false, nil
}

func ownedRow(owner ids.UUID) crmcontracts.WorklistItem {
	id := openapi_types.UUID(owner)
	return crmcontracts.WorklistItem{Owner: &crmcontracts.WorklistOwner{
		Kind: crmcontracts.WorklistOwnerKindWorklistOwnerUser, Id: &id,
	}}
}

func TestAPageIsNamedInOneBatchAcrossItsThreeLists(t *testing.T) {
	lena, ole, ada := ids.NewV7(), ids.NewV7(), ids.NewV7()
	reads := 0
	svc := NewService(stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock).
		WithTeammates(countedRoster{reads: &reads, members: []TeamMember{
			{UserID: lena, DisplayName: "Lena"}, {UserID: ole, DisplayName: "Ole"}, {UserID: ada, DisplayName: "Ada"},
		}})
	optional := []crmcontracts.WorklistItem{ownedRow(ada)}
	page := crmcontracts.Worklist{
		Queue:                 []crmcontracts.WorklistItem{ownedRow(lena)},
		Focus:                 &crmcontracts.WorklistFocus{Items: []crmcontracts.WorklistItem{ownedRow(ole)}},
		OptionalRelationships: &optional,
	}

	if err := svc.nameWorklistRows(pageReader(), &page, nil); err != nil {
		t.Fatalf("naming: %v", err)
	}
	if reads != 1 {
		t.Errorf("the roster was read %d times, want once for the whole page", reads)
	}
	for list, row := range map[string]crmcontracts.WorklistItem{
		"queue": page.Queue[0], "focus": page.Focus.Items[0], "optional": optional[0],
	} {
		if row.Owner.Label == nil {
			t.Errorf("the %s row lost its owner's name", list)
		}
	}
	if *page.Focus.Items[0].Owner.Label != "Ole" || *optional[0].Owner.Label != "Ada" {
		t.Errorf("a name landed on the wrong list's row")
	}
}
