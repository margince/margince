// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What the sales goal and Progress routes refuse, and that a refusal leaves the
// plan as it was.

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// goalMonthsOf is twelve months of value, with the month at gap left unset when
// gap is within the year.
func goalMonthsOf(value int64, gap int) []any {
	months := make([]any, 12)
	for i := range months {
		if i != gap {
			months[i] = value
		}
	}
	return months
}

func TestASalesGoalPlanRefusesWhatItCannotKeep(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	owner := signedInUser(t, e)
	year := clocktest.Now(t).Year()
	plan := fmt.Sprintf("/v1/analytics/sales-goals/%s/%d", owner, year)
	agreed := AnyMap{"reason": "Agreed", "confirm_agreed": true}

	if status, _ := planWrite(t, e, "POST", plan+"/publish", 0, agreed); status != http.StatusUnprocessableEntity {
		t.Errorf("publishing a plan with no draft → %d, want 422", status)
	}
	refusedDrafts := map[string]AnyMap{
		"eleven months":     {"months": goalMonthsOf(1, 11)[:11], "reason": "Short"},
		"a negative month":  {"months": goalMonthsOf(-1, 12), "reason": "Negative"},
		"no reason":         {"months": goalMonthsOf(1, 12), "reason": ""},
		"a negative year":   {"months": goalMonthsOf(1, 12), "reason": "Negative", "declared_total_minor": -1},
		"an unknown origin": {"months": goalMonthsOf(1, 12), "reason": "Origin", "origin": "guessed"},
	}
	for name, draft := range refusedDrafts {
		if status, _ := planWrite(t, e, "PUT", plan+"/draft", 0, draft); status != http.StatusUnprocessableEntity {
			t.Errorf("a draft with %s → %d, want 422", name, status)
		}
	}
	far := fmt.Sprintf("/v1/analytics/sales-goals/%s/%d/draft", owner, 1999)
	if status, _ := planWrite(t, e, "PUT", far, 0, AnyMap{"months": goalMonthsOf(1, 12), "reason": "Old"}); status != http.StatusUnprocessableEntity {
		t.Errorf("a draft for 1999 → %d, want 422", status)
	}

	unpublishable := map[string]AnyMap{
		"a month not set": {"months": goalMonthsOf(1, 3), "reason": "Gap"},
		"no goal at all":  {"months": goalMonthsOf(0, 12), "reason": "Zero"},
	}
	version := int64(0)
	for name, draft := range unpublishable {
		status, saved := planWrite(t, e, "PUT", plan+"/draft", version, draft)
		if status != http.StatusOK {
			t.Fatalf("saving a draft with %s → %d", name, status)
		}
		version = saved.Version
		if status, _ := planWrite(t, e, "POST", plan+"/publish", version, AnyMap{"reason": name, "confirm_agreed": true}); status != http.StatusUnprocessableEntity {
			t.Errorf("publishing a draft with %s → %d, want 422", name, status)
		}
	}
	if status, _ := planWrite(t, e, "POST", plan+"/publish", version, AnyMap{"reason": "Unconfirmed", "confirm_agreed": false}); status != http.StatusUnprocessableEntity {
		t.Errorf("publishing without confirming the goals were agreed → %d, want 422", status)
	}
	if status, _ := planWrite(t, e, "POST", plan+"/withdraw", version, AnyMap{"reason": "Nothing published"}); status != http.StatusUnprocessableEntity {
		t.Errorf("withdrawing a plan never published → %d, want 422", status)
	}

	var kept crmcontracts.SalesGoalPlan
	mustCall(t, e, "GET", plan, nil, http.StatusOK, &kept)
	if kept.Version != version || kept.Published != nil || kept.Draft == nil {
		t.Errorf("after the refusals the plan is %+v, want the last draft at version %d and nothing published", kept, version)
	}
}

func TestProgressRefusesAPeriodOrScopeItCannotRead(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	year := clocktest.Now(t).Year()
	for name, query := range map[string]string{
		"a month with no month named": fmt.Sprintf("period=month&year=%d", year),
		"a year out of range":         "period=year&year=1999",
		"a thirteenth month":          fmt.Sprintf("period=month&year=%d&month=13", year),
		"a team with no team named":   fmt.Sprintf("period=year&year=%d&scope_kind=team", year),
	} {
		if status := e.Call(t, "GET", "/v1/analytics/progress?"+query, nil, nil, nil); status != http.StatusUnprocessableEntity {
			t.Errorf("Progress for %s → %d, want 422", name, status)
		}
	}
}

// While individual performance is off, Progress and the open deals behind it
// refuse a colleague's scope alike. The reader's own deals still answer.
func TestProgressAndItsDealsRefuseAColleagueWhileIndividualPerformanceIsOff(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	colleague := seedColleague(t, e)
	year := clocktest.Now(t).Year()
	mustCall(t, e, http.MethodPatch, "/v1/installation/settings", AnyMap{individualPerformanceKey: false}, http.StatusOK, nil)
	for _, path := range []string{
		fmt.Sprintf("/v1/analytics/progress?period=year&year=%d&scope_kind=owner&scope_id=%s", year, colleague),
		"/v1/analytics/progress/deals?scope_kind=owner&scope_id=" + colleague,
	} {
		var problem struct {
			Code string `json:"code"`
		}
		if status := e.Call(t, http.MethodGet, path, nil, nil, &problem); status != http.StatusForbidden || problem.Code != "individual_performance_off" {
			t.Errorf("GET %s with the switch off = %d %q, want 403 individual_performance_off", path, status, problem.Code)
		}
	}
	mustCall(t, e, http.MethodGet, "/v1/analytics/progress/deals", nil, http.StatusOK, nil)
}

// A team's open deals name each member's work one by one. While individual
// performance is off the team list is refused, and the reader's own answers.
func TestProgressRefusesATeamsDealsWhileIndividualPerformanceIsOff(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	owner := signedInUser(t, e)
	colleague := seedColleague(t, e)
	stages := apptest.DiscoverSeededPipeline(t, e)
	team := createdRecord(t, e, "/v1/teams", AnyMap{"name": "Field sales"})
	for _, member := range []string{owner, colleague} {
		mustCall(t, e, http.MethodPut, "/v1/teams/"+team+"/members/"+member, nil, http.StatusNoContent, nil)
	}
	theirs := createdID(t, e, "/v1/deals", AnyMap{
		"name": "The colleague's deal", "pipeline_id": stages.PipelineID, "stage_id": stages.Open,
		"source": "manual", "amount_minor": 40_000, "currency": "EUR", "owner_id": colleague,
	})
	teamDeals := "/v1/analytics/progress/deals?scope_kind=team&scope_id=" + team
	var work crmcontracts.SalesProgressDealPage
	mustCall(t, e, http.MethodGet, teamDeals, nil, http.StatusOK, &work)
	if dealWorkOf(work, theirs) == nil {
		t.Fatalf("with the switch on the team list misses the colleague's deal: %+v", work.Data)
	}
	mustCall(t, e, http.MethodPatch, "/v1/installation/settings", AnyMap{individualPerformanceKey: false}, http.StatusOK, nil)
	var problem struct {
		Code string `json:"code"`
	}
	if status := e.Call(t, http.MethodGet, teamDeals, nil, nil, &problem); status != http.StatusForbidden || problem.Code != "individual_performance_off" {
		t.Errorf("GET %s with the switch off = %d %q, want 403 individual_performance_off", teamDeals, status, problem.Code)
	}
	mustCall(t, e, http.MethodGet, "/v1/analytics/progress/deals", nil, http.StatusOK, nil)
}
