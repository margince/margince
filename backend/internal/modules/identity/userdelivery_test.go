// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func TestClearingTheTeamFallsBackToMyWork(t *testing.T) {
	team := ids.NewV7()
	for _, scope := range []string{RoutineTeam, RoutineCombined} {
		before := Delivery{Scope: &scope, TeamID: &team}
		after := merged(before, Delivery{ClearTeam: true})
		if after.TeamID != nil {
			t.Errorf("scope %s: the team stayed %v after a clear", scope, *after.TeamID)
		}
		if after.ScopeChoice() != RoutinePersonal {
			t.Errorf("scope %s: a clear left the scope %s, wanted personal", scope, after.ScopeChoice())
		}
	}

	unchosen := merged(Delivery{TeamID: &team}, Delivery{ClearTeam: true})
	if unchosen.TeamID != nil || unchosen.Scope != nil {
		t.Errorf("a seat that never chose a scope got team %v, scope %v; wanted both unset", unchosen.TeamID, unchosen.Scope)
	}
}

func TestAnOmittedTeamIsNotAClear(t *testing.T) {
	team := ids.NewV7()
	after := merged(Delivery{TeamID: &team}, Delivery{})
	if after.TeamID == nil || *after.TeamID != team {
		t.Errorf("a save without the team moved it to %v", after.TeamID)
	}
}

func TestATeamAndItsClearTogetherAreRefused(t *testing.T) {
	team := ids.NewV7()
	err := checkDelivery(Delivery{TeamID: &team, ClearTeam: true})
	var fault *values.ParseError
	if !errors.As(err, &fault) || fault.Field != "clear_routine_mail_team" {
		t.Fatalf("a team sent with its clear answered %v, wanted a refusal on clear_routine_mail_team", err)
	}
}

func TestTheClearFlagReachesTheService(t *testing.T) {
	cleared := true
	if !deliveryFromWire(crmcontracts.BriefDelivery{ClearRoutineMailTeam: &cleared}).ClearTeam {
		t.Error("clear_routine_mail_team: true did not reach the save")
	}
	if deliveryFromWire(crmcontracts.BriefDelivery{}).ClearTeam {
		t.Error("an omitted clear_routine_mail_team cleared the team")
	}
}
