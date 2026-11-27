// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The daily freeze ranks the deals a team works to meet its quarter goal.
// GET /forecast/focus reads that stored list under the reader's own lens.

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// focusFixture is a team whose seller has a published goal and two open
// deals, and a second team whose seller has none. Both are captured daily,
// and the freeze has run once.
type focusFixture struct {
	e                *apptest.AppEnv
	goalTeam, noGoal string
	big, small       string
}

func freezeFocus(t *testing.T) focusFixture {
	t.Helper()
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	now := clocktest.Now(t)
	me := signedInUser(t, e)
	publishEvenGoals(t, e, me, now.Year())
	colleague := addColleagues(t, e, "focus", 1)[0]
	out := focusFixture{e: e, goalTeam: teamWithMembers(t, e, me), noGoal: createdRecord(t, e, "/v1/teams", AnyMap{"name": "Partner sales"})}
	if status := e.Call(t, http.MethodPut, "/v1/teams/"+out.noGoal+"/members/"+colleague, nil, nil, nil); status >= http.StatusBadRequest {
		t.Fatalf("adding the colleague to the second team = %d", status)
	}

	stages := apptest.DiscoverSeededPipeline(t, e)
	today := now.UTC().Format(time.DateOnly)
	deal := func(name string, amount int64) string {
		return createdID(t, e, "/v1/deals", AnyMap{
			"name": name, "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "owner_id": me,
			"source": "manual", "amount_minor": amount, "currency": "EUR", "expected_close_date": today,
		})
	}
	out.big, out.small = deal("Big renewal", 300_000), deal("Small add-on", 120_000)
	createdID(t, e, "/v1/deals", AnyMap{
		"name": "Colleague deal", "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "owner_id": colleague,
		"source": "manual", "amount_minor": 500_000, "currency": "EUR", "expected_close_date": today,
	})

	captures := []crmcontracts.ReportingCaptureContext{}
	for _, team := range []string{out.goalTeam, out.noGoal} {
		parsed, err := ids.Parse(team)
		if err != nil {
			t.Fatal(err)
		}
		id := openapi_types.UUID(parsed)
		captures = append(captures, crmcontracts.ReportingCaptureContext{Scope: crmcontracts.ReportingScope{Kind: "team", Id: &id}})
	}
	var framework crmcontracts.ReportingFramework
	mustCall(t, e, http.MethodGet, "/v1/analytics/framework", nil, http.StatusOK, &framework)
	input := crmcontracts.ReportingFrameworkInput{
		Template: "sales", Reason: "Capture both teams", Qualification: []crmcontracts.ReportingQualification{}, CaptureContexts: captures,
	}
	if status := e.Call(t, http.MethodPut, "/v1/analytics/framework", input,
		map[string]string{"If-Match": strconv.FormatInt(framework.Version, 10)}, nil); status != http.StatusOK {
		t.Fatalf("publishing the capture contexts = %d", status)
	}
	if err := compose.FreezeForecastsForTest(t.Context(), e.Pool, func() time.Time { return now }); err != nil {
		t.Fatalf("running the daily freeze: %v", err)
	}
	return out
}

func readFocus(t *testing.T, e *apptest.AppEnv, query string) map[string]crmcontracts.ForecastFocusTeam {
	t.Helper()
	var focus crmcontracts.ForecastFocus
	mustCall(t, e, http.MethodGet, "/v1/forecast/focus"+query, nil, http.StatusOK, &focus)
	out := map[string]crmcontracts.ForecastFocusTeam{}
	for _, team := range focus.Teams {
		out[team.TeamId.String()] = team
	}
	return out
}

func TestTheDailyFreezeRanksATeamWithAGoalAndNoTeamWithout(t *testing.T) {
	t.Parallel()
	f := freezeFocus(t)
	teams := readFocus(t, f.e, "")

	ranked, ok := teams[f.goalTeam]
	if !ok || ranked.Status != crmcontracts.ForecastFocusTeamStatusBehind {
		t.Fatalf("the team with a goal reads %+v, want a list behind its goal", ranked)
	}
	if len(ranked.Deals) != 2 || ranked.Deals[0].DealId.String() != f.big || ranked.Deals[1].DealId.String() != f.small {
		t.Fatalf("the list is %+v, want the big deal first and the small one second", ranked.Deals)
	}
	first := ranked.Deals[0]
	if first.Rank != 1 || first.DealName != "Big renewal" || first.WeightedMinor == nil || *first.WeightedMinor <= *ranked.Deals[1].WeightedMinor {
		t.Errorf("the first row is %+v, want rank 1 by the larger weighted amount, named", first)
	}
	if first.Reason.ClosesInDays == nil || *first.Reason.ClosesInDays != 0 {
		t.Errorf("the first reason is %+v, want the deal closing today", first.Reason)
	}

	unranked, ok := teams[f.noGoal]
	if !ok || unranked.Status != crmcontracts.ForecastFocusTeamStatusNoGoal || len(unranked.Deals) != 0 {
		t.Errorf("the team without a goal reads %+v, want no_goal and no deals", unranked)
	}
}

// A seat that measures by team reads its own team's list. Another team is not
// found when named and left out when not, and a masked amount stays masked
// while its deal keeps its rank.
func TestAFocusListKeepsToTheReadersTeamAndItsAmountMask(t *testing.T) {
	t.Parallel()
	f := freezeFocus(t)
	narrowSignedInRole(t, f.e, "focus_team_lens", `{}`)

	if status := f.e.Call(t, http.MethodGet, "/v1/forecast/focus?team="+f.noGoal, nil, nil, nil); status != http.StatusNotFound {
		t.Errorf("another team's list = %d, want 404", status)
	}
	teams := readFocus(t, f.e, "")
	if _, seen := teams[f.noGoal]; seen || len(teams) != 1 {
		t.Errorf("the unnamed read lists %d teams, want only the reader's own", len(teams))
	}
	if own := readFocus(t, f.e, "?team="+f.goalTeam)[f.goalTeam]; len(own.Deals) != 2 || own.Deals[0].WeightedMinor == nil {
		t.Fatalf("the reader's own team reads %+v before the mask, so the mask below would prove nothing", own.Deals)
	}

	maskDealField(t, f.e, "focus_team_lens", "amount_minor", "always")
	masked := readFocus(t, f.e, "?team="+f.goalTeam)[f.goalTeam]
	if len(masked.Deals) != 0 || masked.MaskedCount != 2 {
		t.Errorf("the masked list serves %+v and counts %d masked, want no deal in an order made from "+
			"amounts the reader may not see, and both counted", masked.Deals, masked.MaskedCount)
	}
}

// A seat that may not read sales targets is not shown whether its team meets
// its goal, nor the list made from that goal.
func TestAFocusListNeedsTheGoalToBeReadable(t *testing.T) {
	t.Parallel()
	f := freezeFocus(t)
	if own := readFocus(t, f.e, "")[f.goalTeam]; own.Status != crmcontracts.ForecastFocusTeamStatusBehind || len(own.Deals) != 2 {
		t.Fatalf("before the role narrows the team reads %+v, want its list behind the goal", own)
	}
	narrowSignedInRole(t, f.e, "focus_no_targets", `{"sales_target": {"create": false, "read": false, "update": false, "delete": false}}`)
	if own := readFocus(t, f.e, "")[f.goalTeam]; own.Status != crmcontracts.ForecastFocusTeamStatusNoGoal || len(own.Deals) != 0 {
		t.Errorf("without the goal grant the team reads %s with %d deals, want no_goal and none", own.Status, len(own.Deals))
	}
}

// With individual performance off, a team's goal is withheld as Progress
// withholds it, so even the reader's own team shows no standing and no list.
// A colleague's one-seller team is refused when named, and the unnamed read
// leaves it out and says so.
func TestAFocusListUnderTheSwitchOffWithholdsTheGoal(t *testing.T) {
	t.Parallel()
	f := freezeFocus(t)
	if own := readFocus(t, f.e, "")[f.goalTeam]; len(own.Deals) != 2 {
		t.Fatalf("with the switch on the list reads %+v, want both deals", own.Deals)
	}
	mustCall(t, f.e, http.MethodPatch, "/v1/installation/settings", AnyMap{individualPerformanceKey: false}, http.StatusOK, nil)

	var focus crmcontracts.ForecastFocus
	mustCall(t, f.e, http.MethodGet, "/v1/forecast/focus", nil, http.StatusOK, &focus)
	if !focus.Withheld || len(focus.Teams) != 1 || focus.Teams[0].TeamId.String() != f.goalTeam {
		t.Fatalf("the unnamed read = withheld %v with %+v, want withheld and only the reader's own team", focus.Withheld, focus.Teams)
	}
	if own := focus.Teams[0]; own.Status != crmcontracts.ForecastFocusTeamStatusNoGoal || len(own.Deals) != 0 {
		t.Errorf("the reader's own team reads %s with %d deals, want no_goal and none", own.Status, len(own.Deals))
	}
	var problem struct {
		Code string `json:"code"`
	}
	if status := f.e.Call(t, http.MethodGet, "/v1/forecast/focus?team="+f.noGoal, nil, nil, &problem); status != http.StatusForbidden || problem.Code != "individual_performance_off" {
		t.Errorf("the colleague's team = %d %q, want 403 individual_performance_off", status, problem.Code)
	}
}
