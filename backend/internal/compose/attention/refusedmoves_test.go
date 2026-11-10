// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func activityReader() context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:test", UserID: ids.NewV7(), SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			RoleKeys: []string{"read_only"},
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

func rowWith(action crmcontracts.WorklistMoveAction) crmcontracts.WorklistItem {
	return crmcontracts.WorklistItem{Move: &crmcontracts.WorklistMove{Action: action}}
}

func TestEveryListOnThePageBlocksTheMovesThisReaderMayNotTake(t *testing.T) {
	optional := []crmcontracts.WorklistItem{rowWith(crmcontracts.WorklistMoveActionDraftEmail)}
	page := crmcontracts.Worklist{
		Queue: []crmcontracts.WorklistItem{
			rowWith(crmcontracts.WorklistMoveActionCreateTask),
			rowWith(crmcontracts.WorklistMoveActionOpenTask),
			{},
		},
		OptionalRelationships: &optional,
		Focus:                 &crmcontracts.WorklistFocus{Items: []crmcontracts.WorklistItem{rowWith(crmcontracts.WorklistMoveActionDraftReply)}},
	}
	markRefusedMoves(activityReader(), &page)
	for name, move := range map[string]*crmcontracts.WorklistMove{
		"queue create_task":    page.Queue[0].Move,
		"optional draft_email": (*page.OptionalRelationships)[0].Move,
		"focus draft_reply":    page.Focus.Items[0].Move,
	} {
		if move.BlockedReason == nil {
			t.Errorf("%s was offered live to a reader without activity.create", name)
		}
	}
	if got := page.Queue[1].Move.BlockedReason; got != nil {
		t.Errorf("open_task only reads, and was refused: %q", *got)
	}
}
