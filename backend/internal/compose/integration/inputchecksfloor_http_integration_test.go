// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/assurance"
)

// raiseNoNextStep records the input check's finding on one deal, through the
// writer the check runs.
func raiseNoNextStep(t *testing.T, e *apptest.AppEnv, deal, owner string, amount int64) {
	t.Helper()
	raiseFinding(t, e, assurance.Finding{
		Type: assurance.TypeNoNextStep, SubjectID: deal, Severity: "medium", AffectedMinor: &amount, Currency: "EUR",
	}, owner)
}

// raiseFinding records one finding for owner, through the writer the check runs.
func raiseFinding(t *testing.T, e *apptest.AppEnv, finding assurance.Finding, owner string) {
	t.Helper()
	store := assurance.NewStore(compose.InstallationDB(e.Pool))
	if err := store.InTx(apptest.AdminContext(t, e, AdminPerms), func(ctx context.Context, tx pgx.Tx) error {
		return store.UpsertException(ctx, tx, finding, owner)
	}); err != nil {
		t.Fatalf("raising a finding: %v", err)
	}
}

// addColleagues adds n live reps beside the signed-in seat and answers them.
func addColleagues(t *testing.T, e *apptest.AppEnv, slug string, n int) []string {
	t.Helper()
	out := make([]string, 0, n)
	for i := range n {
		var created struct {
			ID string `json:"id"`
		}
		email := fmt.Sprintf("colleague%d@%s.test", i, slug)
		mustCall(t, e, http.MethodPost, "/v1/users", AnyMap{"email": email, "display_name": "Colleague", "role": "rep"}, http.StatusCreated, &created)
		apptest.SetPasswordByLink(t, e, created.ID, fmt.Sprintf("%s-colleague-%d-password", slug, i))
		out = append(out, created.ID)
	}
	return out
}

// teamOf puts the members on one new team and answers it.
func teamWithMembers(t *testing.T, e *apptest.AppEnv, members ...string) string {
	t.Helper()
	team := createdRecord(t, e, "/v1/teams", AnyMap{"name": "Field sales"})
	for _, member := range members {
		if status := e.Call(t, http.MethodPut, "/v1/teams/"+team+"/members/"+member, nil, nil, nil); status >= http.StatusBadRequest {
			t.Fatalf("adding a member to the team = %d", status)
		}
	}
	return team
}

// twoSellerTeam signs in the workspace's first seat and puts it on one team
// with one colleague. It answers both seats and the team.
func twoSellerTeam(t *testing.T, e *apptest.AppEnv, slug string) (me, colleague, team string) {
	t.Helper()
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	colleague = addColleagues(t, e, slug, 1)[0]
	me = signedInUser(t, e)
	return me, colleague, teamWithMembers(t, e, me, colleague)
}

// The input checks of a team of the reader and one colleague sum each type's
// exposure. Less the reader's own, that is the colleague's. While the switch
// is off the team's list and the whole list are refused; the reader's own is
// not.
func TestIndividualPerformanceOffRefusesATwoMemberTeamsInputChecks(t *testing.T) {
	e := apptest.SetupApp(t)
	me, colleague, team := twoSellerTeam(t, e, "checks-floor")
	stages := apptest.DiscoverSeededPipeline(t, e)
	for index, owner := range []string{me, colleague} {
		deal := createdRecord(t, e, "/v1/deals", AnyMap{
			"name": "Checked deal", "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "source": "manual", "owner_id": owner,
		})
		raiseNoNextStep(t, e, deal, owner, int64(100_000*(index+1)))
	}
	const list = "/v1/forecast/assurance/exceptions"
	teamList := list + "?scope_kind=team&scope_id=" + team
	ownList := list + "?scope_kind=owner&scope_id=" + me
	for _, path := range []string{list, teamList, ownList} {
		if status := e.Call(t, http.MethodGet, path, nil, nil, nil); status != http.StatusOK {
			t.Errorf("with the switch on, %s = %d, want 200", path, status)
		}
	}
	mustCall(t, e, http.MethodPatch, "/v1/installation/settings", AnyMap{individualPerformanceKey: false}, http.StatusOK, nil)
	for _, path := range []string{list, teamList} {
		var problem struct {
			Code string `json:"code"`
		}
		if status := e.Call(t, http.MethodGet, path, nil, nil, &problem); status != http.StatusForbidden || problem.Code != "individual_performance_off" {
			t.Errorf("with the switch off, %s = %d %q, want 403 individual_performance_off", path, status, problem.Code)
		}
	}
	var mine struct {
		Data []struct {
			OwnerID string `json:"owner_id"`
		} `json:"data"`
	}
	if status := e.Call(t, http.MethodGet, ownList, nil, nil, &mine); status != http.StatusOK || len(mine.Data) != 1 || mine.Data[0].OwnerID != me {
		t.Errorf("with the switch off, the reader's own checks = %d %+v, want 200 with the one finding on their deal", status, mine.Data)
	}
}
