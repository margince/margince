// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// issueTeamShare issues a share of one team's forecast and answers its token.
func issueTeamShare(t *testing.T, e *apptest.AppEnv, team string, body AnyMap) string {
	t.Helper()
	body["target"], body["scope_kind"], body["scope_id"] = "someone@example.test", "team", team
	body["expires_at"] = clocktest.Now(t).UTC().Add(24 * time.Hour)
	var issued struct {
		Token string `json:"token"`
	}
	mustCall(t, e, http.MethodPost, "/v1/forecast/shares", body, http.StatusCreated, &issued)
	return issued.Token
}

// A share of a team of the reader and one colleague answers that colleague's
// figures. While the switch is off, opening it and its export are refused.
func TestIndividualPerformanceOffRefusesATwoMemberTeamsShare(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	var colleague struct {
		ID string `json:"id"`
	}
	mustCall(t, e, http.MethodPost, "/v1/users", AnyMap{"email": "colleague@share-floor.test", "display_name": "Colleague", "role": "rep"}, http.StatusCreated, &colleague)
	apptest.SetPasswordByLink(t, e, colleague.ID, "share-floor-colleague-password")
	team := createdRecord(t, e, "/v1/teams", AnyMap{"name": "Field sales"})
	for _, member := range []string{colleague.ID, signedInUser(t, e)} {
		if status := e.Call(t, http.MethodPut, "/v1/teams/"+team+"/members/"+member, nil, nil, nil); status >= http.StatusBadRequest {
			t.Fatalf("adding a member to the team = %d", status)
		}
	}
	teamID, err := ids.Parse(team)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := apptest.FreezeScopeAt(t, e, AdminPerms, forecasting.Scope{Kind: forecasting.ScopeTeam, ID: &teamID}, clocktest.Now(t))
	frozen := issueTeamShare(t, e, team, AnyMap{"kind": "snapshot", "snapshot_id": snapshot})
	live := issueTeamShare(t, e, team, AnyMap{"kind": "live"})
	reads := map[string]string{
		"the snapshot share": "/v1/forecast/shared/" + frozen, "the live share": "/v1/forecast/shared/" + live,
		"the snapshot export": "/v1/forecast/shared/" + frozen + "/export.csv",
	}
	for what, path := range reads {
		if status := e.Call(t, http.MethodGet, path, nil, nil, nil); status != http.StatusOK {
			t.Errorf("with the switch on, %s = %d, want 200", what, status)
		}
	}
	mustCall(t, e, http.MethodPatch, "/v1/installation/settings", AnyMap{individualPerformanceKey: false}, http.StatusOK, nil)
	for what, path := range reads {
		if status := e.Call(t, http.MethodGet, path, nil, nil, nil); status != http.StatusForbidden {
			t.Errorf("with the switch off, %s of a two-member team = %d, want 403", what, status)
			continue
		}
		var problem struct {
			Code string `json:"code"`
		}
		if e.Call(t, http.MethodGet, path, nil, nil, &problem); problem.Code != "individual_performance_off" {
			t.Errorf("with the switch off, %s is refused with %q, want individual_performance_off", what, problem.Code)
		}
	}
}
