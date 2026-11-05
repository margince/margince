// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// asColleague is a second member who may change every deal. The one who sets
// a probability is then never the one who reads it.
func (e *configEnv) asColleague(t *testing.T) (context.Context, ids.UUID) {
	t.Helper()
	colleague := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Kim Colleague')`,
		colleague, "kim-"+colleague.String()+"@config.test"); err != nil {
		t.Fatalf("seeding the colleague: %v", err)
	}
	return principal.WithActor(e.asDealWriter(), principal.Principal{
		Type: principal.PrincipalHuman, ID: principal.HumanIDPrefix + colleague.String(), UserID: colleague,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"}, RowScope: principal.RowScopeAll,
			Objects: map[string]principal.ObjectGrant{"deal": {Read: true, Update: true}},
		},
	}), colleague
}

// setterOf reads the deal as the admin and answers who it says set the
// probability, empty when it names nobody.
func setterOf(t *testing.T, e *configEnv, deal ids.DealID) (string, string) {
	t.Helper()
	read, err := e.store.GetDeal(e.asDealWriter(), deal, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the deal: %v", err)
	}
	if read.WinProbabilitySetByUserId == nil {
		return "", ""
	}
	name := ""
	if read.WinProbabilitySetByName != nil {
		name = *read.WinProbabilitySetByName
	}
	return read.WinProbabilitySetByUserId.String(), name
}

func TestADealNamesTheColleagueWhoSetItsProbability(t *testing.T) {
	e := setupConfigEnv(t)
	deal, _, next := seedOwnedDeal(t, e)
	if id, _ := setterOf(t, e, deal); id != "" {
		t.Fatalf("a deal that follows its stage names %s as the setter, want nobody", id)
	}

	colleagueCtx, colleague := e.asColleague(t)
	set := 70
	if _, err := e.store.UpdateDeal(colleagueCtx, deal, UpdateDealInput{WinProbability: &set}); err != nil {
		t.Fatalf("the colleague setting the probability: %v", err)
	}
	renamed := "Contested, renamed"
	if _, err := e.store.UpdateDeal(e.asDealWriter(), deal, UpdateDealInput{Name: &renamed}); err != nil {
		t.Fatalf("the admin renaming the deal: %v", err)
	}
	if id, name := setterOf(t, e, deal); id != colleague.String() || name != "Kim Colleague" {
		t.Fatalf("after the admin's rename the setter reads %q (%s), want the colleague who set it, Kim Colleague (%s)",
			name, id, colleague)
	}

	again := 55
	if _, err := e.store.UpdateDeal(e.asDealWriter(), deal, UpdateDealInput{WinProbability: &again}); err != nil {
		t.Fatalf("the admin setting the probability: %v", err)
	}
	if id, name := setterOf(t, e, deal); id != e.admin.String() || name != "Admin" {
		t.Fatalf("after the admin's own edit the setter reads %q (%s), want Admin (%s)", name, id, e.admin)
	}

	if _, err := e.store.AdvanceDeal(e.asDealWriter(), deal, AdvanceDealInput{ToStageID: next}); err != nil {
		t.Fatalf("moving the deal: %v", err)
	}
	if id, _ := setterOf(t, e, deal); id != "" {
		t.Fatalf("a deal back on its stage's probability names %s as the setter, want nobody", id)
	}
}
