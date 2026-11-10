// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"context"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func seatWith(seat principal.SeatType, activity principal.ObjectGrant) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:test", UserID: ids.NewV7(), SeatType: seat,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects:  map[string]principal.ObjectGrant{"activity": activity},
			RowScope: principal.RowScopeAll,
		},
	})
}

func TestTheCardsMoveIsBlockedOnTheServedCopyOnly(t *testing.T) {
	filed := crmcontracts.DealStatusCard{Next: &crmcontracts.DealStatusCardMove{Action: "create_task"}}
	served := withRefusedMove(seatWith(principal.SeatFull, principal.ObjectGrant{Read: true}), filed)
	if got := served.Next.BlockedReason; got == nil || *got != "You do not have permission to add tasks" {
		t.Fatalf("a reader without activity.create was served create_task with blocked_reason %v", got)
	}
	if filed.Next.BlockedReason != nil {
		t.Fatal("the refusal reached the card filed for every reader of the deal")
	}
	writer := seatWith(principal.SeatFull, principal.ObjectGrant{Read: true, Create: true})
	if got := withRefusedMove(writer, filed).Next.BlockedReason; got != nil {
		t.Fatalf("a writer was refused create_task: %q", *got)
	}
	if got := withRefusedMove(writer, crmcontracts.DealStatusCard{}).Next; got != nil {
		t.Fatalf("a card with no move was given one: %+v", got)
	}
}

func TestTheCardOffersAReadingMoveToEveryReader(t *testing.T) {
	card := crmcontracts.DealStatusCard{Next: &crmcontracts.DealStatusCardMove{Action: "open_task"}}
	if got := withRefusedMove(seatWith(principal.SeatRead, principal.ObjectGrant{Read: true}), card).Next.BlockedReason; got != nil {
		t.Fatalf("open_task only reads, and was refused: %q", *got)
	}
}
