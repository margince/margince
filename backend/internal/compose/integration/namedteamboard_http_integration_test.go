// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A named team's board lists the seat invited onto it before they sign in,
// marked unmeasured, and drops the seat that has since been deactivated.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type teamBoardMemberWire struct {
	UserID     string `json:"user_id"`
	Activation string `json:"activation"`
}

func inviteOntoTeam(t *testing.T, e *apptest.AppEnv, email, team string) string {
	t.Helper()
	var seat struct {
		ID string `json:"id"`
	}
	mustCall(t, e, "POST", "/v1/users", AnyMap{
		"email": email, "display_name": email, "role": "rep", "team_ids": []string{team},
	}, http.StatusCreated, &seat)
	return seat.ID
}

func TestANamedTeamBoardListsItsInvitedSeatAndNotItsDeactivatedOne(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	var team struct {
		ID string `json:"id"`
	}
	var me struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	mustCall(t, e, "POST", "/v1/teams", AnyMap{"name": "Invite Desk"}, http.StatusCreated, &team)
	mustCall(t, e, "GET", "/v1/me", nil, http.StatusOK, &me)
	mustCall(t, e, "PUT", "/v1/teams/"+team.ID+"/members/"+me.User.ID, nil, http.StatusNoContent, nil)
	invited := inviteOntoTeam(t, e, "arriving@invite-desk.test", team.ID)
	departed := inviteOntoTeam(t, e, "departed@invite-desk.test", team.ID)
	mustCall(t, e, "POST", "/v1/users/"+departed+"/deactivate", nil, http.StatusOK, nil)

	var board struct {
		Members []teamBoardMemberWire `json:"members"`
	}
	mustCall(t, e, "GET", "/v1/worklist/team?team="+team.ID, nil, http.StatusOK, &board)
	activation := map[string]string{}
	for _, member := range board.Members {
		activation[member.UserID] = member.Activation
	}
	if activation[invited] != "invited" {
		t.Errorf("the seat invited onto the team reads %q, want it listed as invited", activation[invited])
	}
	if activation[me.User.ID] != "active" {
		t.Errorf("the signed-in member reads %q, want active", activation[me.User.ID])
	}
	if _, listed := activation[departed]; listed {
		t.Error("a deactivated seat is still on the team's board")
	}
}
