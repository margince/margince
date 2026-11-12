// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Analytics and Progress read one goal: the published monthly sales goals,
// unless a target was set for the very scope and period.

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// publishGoals publishes even monthly goals for owner in year.
func publishGoals(t *testing.T, e *apptest.AppEnv, owner string, year int) {
	t.Helper()
	plan := fmt.Sprintf("/v1/analytics/sales-goals/%s/%d", owner, year)
	origin := crmcontracts.SalesGoalDraftInputOrigin("manual")
	_, saved := planWrite(t, e, "PUT", plan+"/draft", 0, crmcontracts.SalesGoalDraftInput{Months: evenMonths(), Reason: "Agreed in the yearly review", Origin: &origin})
	if status, _ := planWrite(t, e, "POST", plan+"/publish", saved.Version, crmcontracts.SalesGoalPublishInput{Reason: "Agreed in the yearly review", ConfirmAgreed: true}); status != http.StatusOK {
		t.Fatalf("publishing the goals of %s → %d", owner, status)
	}
}

// quarterSalesWon reads sales won this fiscal quarter for one scope.
func quarterSalesWon(t *testing.T, e *apptest.AppEnv, kind, id string) crmcontracts.ReportingMetric {
	t.Helper()
	var evaluation crmcontracts.ReportingEvaluation
	query := fmt.Sprintf("period=this_quarter&target_basis=fiscal_quarter&close_window=all_open&metrics=bookings_won&scope_kind=%s&scope_id=%s", kind, id)
	mustCall(t, e, "GET", "/v1/analytics/evaluate?"+query, nil, http.StatusOK, &evaluation)
	if len(evaluation.Metrics) != 1 {
		t.Fatalf("the evaluation holds %d metrics, want sales won alone", len(evaluation.Metrics))
	}
	return evaluation.Metrics[0]
}

func TestAnalyticsHoldsAQuarterAgainstThePublishedSalesGoals(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	owner := signedInUser(t, e)
	now := clocktest.Now(t)

	if got := quarterSalesWon(t, e, "owner", owner); got.Target != nil {
		t.Fatalf("a seller with no goals stands against %v, want no target", *got.Target)
	}
	publishGoals(t, e, owner, now.Year())

	got := quarterSalesWon(t, e, "owner", owner)
	if got.Target == nil || int64(*got.Target) != 3*monthlyGoal {
		t.Fatalf("the quarter target = %v, want three published months, %d", got.Target, 3*monthlyGoal)
	}
	if got.TargetSource == nil || *got.TargetSource != crmcontracts.ReportingMetricTargetSourceSalesGoals || got.TargetRevision != nil {
		t.Errorf("the quarter target names source %v and revision %v, want sales_goals and no revision", got.TargetSource, got.TargetRevision)
	}

	team := createdRecord(t, e, "/v1/teams", AnyMap{"name": "Field sales"})
	if status := e.Call(t, http.MethodPut, "/v1/teams/"+team+"/members/"+owner, nil, nil, nil); status >= http.StatusBadRequest {
		t.Fatalf("adding the seller to the team → %d", status)
	}
	if got := quarterSalesWon(t, e, "team", team); got.Target == nil || int64(*got.Target) != 3*monthlyGoal {
		t.Errorf("the team's quarter target = %v, want its one seller's three months", got.Target)
	}

	quarterStart := now.AddDate(0, 0, 1-now.Day()).AddDate(0, -((int(now.Month())-1)%3), 0)
	explicit := AnyMap{
		"metric": "bookings_won", "scope": AnyMap{"kind": "team", "id": team}, "period_kind": "fiscal_quarter",
		"period_start": fmt.Sprintf("%d-%02d-01", quarterStart.Year(), int(quarterStart.Month())), "value": 7 * monthlyGoal, "reason": "Set for the team",
	}
	var target crmcontracts.ReportingTarget
	mustCall(t, e, http.MethodPost, "/v1/analytics/targets", explicit, http.StatusCreated, &target)
	got = quarterSalesWon(t, e, "team", team)
	if got.Target == nil || int64(*got.Target) != 7*monthlyGoal || got.TargetSource == nil || *got.TargetSource != crmcontracts.ReportingMetricTargetSourceTarget {
		t.Errorf("with a team target set, the quarter stands against %v from %v, want the target itself", got.Target, got.TargetSource)
	}

	mustCall(t, e, "GET", "/v1/analytics/targets/"+target.Id.String(), nil, http.StatusOK, &target)
	if target.AllocatedValue == nil || *target.AllocatedValue != 3*monthlyGoal {
		t.Errorf("the team target counts %v as allocated, want its seller's goals for the quarter", target.AllocatedValue)
	}

	var typed, planned crmcontracts.ReportingTargetList
	mustCall(t, e, "GET", "/v1/analytics/targets?plan_owned=false&limit=100", nil, http.StatusOK, &typed)
	mustCall(t, e, "GET", "/v1/analytics/targets?plan_owned=true&limit=100", nil, http.StatusOK, &planned)
	if len(typed.Data) != 1 || typed.Data[0].Id != target.Id || len(planned.Data) != 12 {
		t.Errorf("the target list splits into %d typed and %d plan-owned, want the team target and twelve months", len(typed.Data), len(planned.Data))
	}
}

func TestASellerReadsNoTeamInProgress(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	const email, password = "seller@team-progress.test", "team-progress-seller-password"
	var user struct {
		ID string `json:"id"`
	}
	mustCall(t, e, http.MethodPost, "/v1/users", AnyMap{"email": email, "display_name": "Sam Seller", "role": "rep"}, http.StatusCreated, &user)
	apptest.SetPasswordByLink(t, e, user.ID, password)
	team := createdRecord(t, e, "/v1/teams", AnyMap{"name": "Field sales"})
	for _, member := range []string{user.ID, signedInUser(t, e)} {
		if status := e.Call(t, http.MethodPut, "/v1/teams/"+team+"/members/"+member, nil, nil, nil); status >= http.StatusBadRequest {
			t.Fatalf("adding a member to the team → %d", status)
		}
	}
	year := clocktest.Now(t).Year()
	query := fmt.Sprintf("period=year&year=%d&scope_kind=team&scope_id=%s", year, team)

	if manager := readProgress(t, e, query); manager.Scope.Kind != "team" {
		t.Errorf("the manager's team reading is about %s", manager.Scope.Kind)
	}
	seller := e.AsSecondSeat(t, email, password)
	if status := seller.Call(t, "GET", "/v1/analytics/progress?"+query, nil, nil, nil); status != http.StatusNotFound {
		t.Errorf("a seller reading their team's progress → %d, want 404", status)
	}
	var frame crmcontracts.AnalyticsContext
	mustCall(t, seller, "GET", "/v1/analytics/context", nil, http.StatusOK, &frame)
	for _, scope := range frame.AllowedScopes {
		if own := scope.Kind == "owner"; scope.Measurable == nil || *scope.Measurable != own {
			t.Errorf("the seller's frame marks %s %q measurable=%v, want only their own figures", scope.Kind, scope.Label, scope.Measurable)
		}
	}
}

func TestTheGoalRosterNamesADraftsYearlySum(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	owner := signedInUser(t, e)
	year := clocktest.Now(t).Year()
	origin := crmcontracts.SalesGoalDraftInputOrigin("manual")
	draft := crmcontracts.SalesGoalDraftInput{Months: evenMonths(), Reason: "Proposed for the yearly review", Origin: &origin}
	if status, _ := planWrite(t, e, "PUT", fmt.Sprintf("/v1/analytics/sales-goals/%s/%d/draft", owner, year), 0, draft); status != http.StatusOK {
		t.Fatalf("saving the draft → %d", status)
	}
	var roster crmcontracts.SalesGoalRoster
	mustCall(t, e, "GET", fmt.Sprintf("/v1/analytics/sales-goals?year=%d", year), nil, http.StatusOK, &roster)
	for _, row := range roster.Data {
		if row.OwnerId.String() != owner {
			continue
		}
		if row.TotalMinor != nil || row.DraftTotalMinor == nil || *row.DraftTotalMinor != yearlyGoal {
			t.Errorf("the roster row = published %v, draft %v; want nothing published and a draft of %d", row.TotalMinor, row.DraftTotalMinor, yearlyGoal)
		}
		return
	}
	t.Fatal("the roster does not list the signed-in manager")
}
