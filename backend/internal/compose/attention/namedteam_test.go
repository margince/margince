// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// namedRoster is a named team's membership reader over a fixed roster.
type namedRoster []TeamMember

func (n namedRoster) MembersOfTeam(context.Context, ids.UUID) ([]TeamMember, bool, error) {
	return []TeamMember(n), false, nil
}

var theInvitee = ids.MustParse("01a05500-0000-7000-8000-0000000000d0")

func boardRowsByID(board crmcontracts.TeamBoard) map[openapi_types.UUID]crmcontracts.TeamBoardMember {
	rows := map[openapi_types.UUID]crmcontracts.TeamBoardMember{}
	for _, member := range board.Members {
		rows[member.UserId] = member
	}
	return rows
}

func TestANamedTeamListsAnInvitedSeatWithoutMeasuringIt(t *testing.T) {
	t.Parallel()
	svc := boardService()
	promises := &promisesSaying{per: map[ids.UUID]int{}}
	svc.promiseLoad = promises
	svc.overdueLoad = overdueSaying{theColleague: 2, theInvitee: 3}
	svc.WithNamedTeams(namedRoster{
		{UserID: theColleague, DisplayName: "Bb Colleague"},
		{UserID: theInvitee, DisplayName: "Aa Invitee", Invited: true},
	})
	board, err := svc.NamedTeamBoard(boardReaderAt(principal.RowScopeTeam), ids.NewV7())
	if err != nil {
		t.Fatal(err)
	}
	if len(board.Members) != 2 || board.Members[0].DisplayName != "Aa Invitee" {
		t.Fatalf("the invited seat must appear, ordered by name like any other: %+v", board.Members)
	}
	rows := boardRowsByID(board)
	invitee := rows[openapi_types.UUID(theInvitee)]
	if invitee.Activation != crmcontracts.TeamBoardMemberActivationInvited || invitee.Counts != (crmcontracts.TeamBoardCounts{}) {
		t.Errorf("an invited seat is unmeasured and must say so: %+v", invitee)
	}
	colleague := rows[openapi_types.UUID(theColleague)]
	if colleague.Activation != crmcontracts.TeamBoardMemberActivationActive || colleague.Counts.Overdue != 2 {
		t.Errorf("an active teammate keeps their measured load: %+v", colleague)
	}
	for _, owner := range promises.asked {
		if owner == theInvitee {
			t.Error("the invited seat's promises were counted, though its workload is not measured")
		}
	}
}

func TestTheCallersOwnTeamBoardMarksEveryoneActive(t *testing.T) {
	t.Parallel()
	board, err := boardService(theTeam...).TeamBoard(boardReaderAt(principal.RowScopeTeam))
	if err != nil {
		t.Fatal(err)
	}
	for _, member := range board.Members {
		if member.Activation != crmcontracts.TeamBoardMemberActivationActive {
			t.Errorf("%s is on a live roster but reads as %s", member.DisplayName, member.Activation)
		}
	}
}
