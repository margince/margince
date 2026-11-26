// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A seller's goal plan written through its routes, and the Progress reading
// that compares won and open deals against the goals it publishes.

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// A month of goal in EUR cents, and the year total twelve of them make.
const (
	monthlyGoal = int64(100_000_00)
	yearlyGoal  = 12 * monthlyGoal
)

func signedInUser(t *testing.T, e *apptest.AppEnv) string {
	t.Helper()
	var me crmcontracts.MeResponse
	mustCall(t, e, "GET", "/v1/me", nil, http.StatusOK, &me)
	return me.User.Id.String()
}

func evenMonths() crmcontracts.SalesGoalMonths {
	months := make(crmcontracts.SalesGoalMonths, 12)
	for i := range months {
		amount := monthlyGoal
		months[i] = &amount
	}
	return months
}

// planWrite sends one write of the plan at version and returns the plan it answers.
//
//craft:ignore naked-any body is the input shape each plan route takes
func planWrite(t *testing.T, e *apptest.AppEnv, method, path string, version int64, body any) (int, crmcontracts.SalesGoalPlan) {
	t.Helper()
	var plan crmcontracts.SalesGoalPlan
	// One key per body, so a refused body is never replayed for the next one.
	headers := map[string]string{
		"If-Match":        strconv.FormatInt(version, 10),
		"Idempotency-Key": fmt.Sprintf("%x", sha256.Sum256(fmt.Appendf(nil, "%s %s %d %v", method, path, version, body))),
	}
	return e.Call(t, method, path, body, headers, &plan), plan
}

func TestASalesGoalPlanIsDraftedPublishedAndWithdrawnOverTheWire(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	owner := signedInUser(t, e)
	year := clocktest.Now(t).Year()
	plan := fmt.Sprintf("/v1/analytics/sales-goals/%s/%d", owner, year)

	var roster crmcontracts.SalesGoalRoster
	mustCall(t, e, "GET", fmt.Sprintf("/v1/analytics/sales-goals?year=%d", year), nil, http.StatusOK, &roster)
	if !rosterHolds(roster, owner) {
		t.Fatalf("the roster of %d does not list the signed-in manager: %+v", year, roster.Data)
	}

	var empty crmcontracts.SalesGoalPlan
	mustCall(t, e, "GET", plan, nil, http.StatusOK, &empty)
	if empty.Version != 0 || empty.Draft != nil || empty.Published != nil || !empty.Capabilities.Edit {
		t.Fatalf("a plan never written = %+v, want version 0, nothing drafted or published, and editable", empty)
	}

	origin := crmcontracts.SalesGoalDraftInputOrigin("manual")
	total := yearlyGoal
	draft := crmcontracts.SalesGoalDraftInput{Months: evenMonths(), DeclaredTotalMinor: &total, Reason: "Agreed in the yearly review", Origin: &origin}
	status, saved := planWrite(t, e, "PUT", plan+"/draft", 0, draft)
	if status != http.StatusOK || saved.Draft == nil {
		t.Fatalf("saving the first draft → %d %+v", status, saved)
	}
	if n := liveMonthTargets(t, e, owner); n != 0 {
		t.Errorf("a draft wrote %d monthly targets, want none until it is published", n)
	}
	stale := map[string]string{"If-Match": "0", "Idempotency-Key": "stale-draft"}
	if status := e.Call(t, "PUT", plan+"/draft", draft, stale, nil); status != http.StatusConflict {
		t.Errorf("a draft sent at the version before it → %d, want 409", status)
	}

	status, discarded := planWrite(t, e, "DELETE", plan+"/draft", saved.Version, nil)
	if status != http.StatusOK || discarded.Draft != nil || discarded.History[len(discarded.History)-1].Kind != "discarded" {
		t.Fatalf("discarding the draft → %d %+v, want no draft and a discard in the history", status, discarded)
	}

	offTotal := yearlyGoal + monthlyGoal
	uneven := draft
	uneven.DeclaredTotalMinor = &offTotal
	_, saved = planWrite(t, e, "PUT", plan+"/draft", discarded.Version, uneven)
	if status, _ := planWrite(t, e, "POST", plan+"/publish", saved.Version,
		crmcontracts.SalesGoalPublishInput{Reason: "Agreed", ConfirmAgreed: true}); status != http.StatusUnprocessableEntity {
		t.Errorf("publishing months that miss the declared year → %d, want 422", status)
	}
	if n := liveMonthTargets(t, e, owner); n != 0 {
		t.Errorf("a refused publication wrote %d monthly targets", n)
	}
	status, saved = planWrite(t, e, "PUT", plan+"/draft", saved.Version, draft)
	if status != http.StatusOK {
		t.Fatalf("saving the draft again → %d", status)
	}

	status, published := planWrite(t, e, "POST", plan+"/publish", saved.Version,
		crmcontracts.SalesGoalPublishInput{Reason: "Agreed in the yearly review", ConfirmAgreed: true})
	if status != http.StatusOK || published.Published == nil || published.Draft != nil {
		t.Fatalf("publishing → %d %+v", status, published)
	}
	if got := monthsTotal(published.LiveMonths); got != yearlyGoal || liveMonthTargets(t, e, owner) != 12 {
		t.Errorf("Progress reads %d of live goals, want the published %d as twelve monthly targets", got, yearlyGoal)
	}

	progress := readProgress(t, e, fmt.Sprintf("period=year&year=%d&scope_kind=owner&scope_id=%s", year, owner))
	if progress.Goal.AmountMinor == nil || *progress.Goal.AmountMinor != yearlyGoal || progress.Goal.MonthsSet != 12 {
		t.Errorf("the yearly Progress goal = %+v, want the published %d over 12 months", progress.Goal, yearlyGoal)
	}

	status, withdrawn := planWrite(t, e, "POST", plan+"/withdraw", published.Version, crmcontracts.SalesGoalReasonInput{Reason: "Left the team"})
	if status != http.StatusOK || withdrawn.Published != nil {
		t.Fatalf("withdrawing → %d %+v", status, withdrawn)
	}
	if len(withdrawn.History) < 4 {
		t.Errorf("the history holds %d events, want the save, discard, publish and withdraw", len(withdrawn.History))
	}
	if n := liveMonthTargets(t, e, owner); n != 0 {
		t.Errorf("a withdrawn plan left %d live monthly targets", n)
	}
	var retired crmcontracts.ReportingTargetList
	mustCall(t, e, "GET", fmt.Sprintf("/v1/analytics/targets?retired=true&period_start=%d-01-01", year), nil, http.StatusOK, &retired)
	if len(retired.Data) != 1 {
		t.Fatalf("retired January targets = %d, want the one the plan wrote", len(retired.Data))
	}
	january := retired.Data[0]
	version := map[string]string{"If-Match": strconv.FormatInt(january.Version, 10)}
	if status := e.Call(t, http.MethodPatch, "/v1/analytics/targets/"+january.Id.String(), monthTarget(owner, year), version, nil); status != http.StatusOK {
		t.Errorf("changing a month a withdrawn plan held → %d, want 200", status)
	}
}

// monthTarget is a January sales goal written past the plan, on the target route.
func monthTarget(owner string, year int) AnyMap {
	return AnyMap{
		"metric": "bookings_won", "scope": AnyMap{"kind": "owner", "id": owner},
		"period_kind": "month", "period_start": fmt.Sprintf("%d-01-01", year), "value": 1, "reason": "Bypass the plan",
	}
}

// liveMonthTargets counts the owner's live monthly targets, which a published
// plan writes and nothing else should.
func liveMonthTargets(t *testing.T, e *apptest.AppEnv, owner string) int {
	t.Helper()
	var list crmcontracts.ReportingTargetList
	mustCall(t, e, "GET", "/v1/analytics/targets?retired=false&limit=100", nil, http.StatusOK, &list)
	count := 0
	for _, target := range list.Data {
		scope := target.Definition.Scope
		if scope.Kind == "owner" && scope.Id != nil && scope.Id.String() == owner {
			count++
		}
	}
	return count
}

func TestProgressSplitsWonValueByProductKindAndListsTheOpenWork(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	owner := signedInUser(t, e)
	now := clocktest.Now(t)
	stages := apptest.DiscoverSeededPipeline(t, e)

	licence := createdRecord(t, e, "/v1/products", AnyMap{
		"source": "manual", "name": "Licence", "unit_price_minor": 200_000, "currency": "EUR", "component_kind": "software",
	})
	rollout := createdRecord(t, e, "/v1/products", AnyMap{
		"source": "manual", "name": "Rollout", "unit_price_minor": 100_000, "currency": "EUR", "component_kind": "implementation",
	})
	won := createdID(t, e, "/v1/deals", AnyMap{
		"name": "Won with products", "pipeline_id": stages.PipelineID, "stage_id": stages.Open,
		"source": "manual", "amount_minor": 300_000, "currency": "EUR",
	})
	allocate(t, e, won, AnyMap{"product_id": licence, "amount_minor": 200_000}, AnyMap{"product_id": rollout, "amount_minor": 100_000})
	mustCall(t, e, "POST", "/v1/deals/"+won+"/advance",
		AnyMap{"to_stage_id": stages.Won, "won_without_contract_reason": "verbal"}, http.StatusOK, nil)

	late := createdID(t, e, "/v1/deals", AnyMap{
		"name": "Open with no close date", "pipeline_id": stages.PipelineID, "stage_id": stages.Open,
		"source": "manual", "amount_minor": 50_000, "currency": "EUR",
	})
	allocate(t, e, late, AnyMap{"product_id": licence, "amount_minor": 50_000})
	fileClosingTask(t, e, "Send the revised quote", "deal", late)

	year := readProgress(t, e, fmt.Sprintf("period=year&year=%d", now.Year()))
	if year.Closed.AmountMinor == nil || *year.Closed.AmountMinor < 300_000 {
		t.Errorf("closed this year = %+v, want at least the won 300000", year.Closed)
	}
	if year.Categories.ProductMinor < 200_000 || year.Categories.ServiceMinor < 100_000 {
		t.Errorf("the won mix = %+v, want the licence as product and the rollout as service", year.Categories)
	}
	if len(year.Months) != 12 {
		t.Errorf("the yearly reading has %d months, want 12", len(year.Months))
	}

	month := readProgress(t, e, fmt.Sprintf("period=month&year=%d&month=%d", now.Year(), int(now.Month())))
	if month.Period.Month == nil || *month.Period.Month != int(now.Month()) {
		t.Errorf("the monthly reading is for %+v, want month %d", month.Period, now.Month())
	}

	team := createdRecord(t, e, "/v1/teams", AnyMap{"name": "Field sales"})
	mustCall(t, e, http.MethodPut, "/v1/teams/"+team+"/members/"+owner, nil, http.StatusNoContent, nil)
	byTeam := readProgress(t, e, fmt.Sprintf("period=year&year=%d&scope_kind=team&scope_id=%s", now.Year(), team))
	if len(byTeam.Members) == 0 && !byTeam.MembersWithheld {
		t.Errorf("the team reading names no member and withholds none: %+v", byTeam)
	}

	var work crmcontracts.SalesProgressDealPage
	mustCall(t, e, "GET", "/v1/analytics/progress/deals?limit=10", nil, http.StatusOK, &work)
	open := dealWorkOf(work, late)
	if open == nil {
		t.Fatalf("the open deal is not in the Progress work list: %+v", work.Data)
	}
	if open.NextTask == nil || open.NextTask.Subject != "Send the revised quote" {
		t.Errorf("the open deal's next task = %+v, want the quote task", open.NextTask)
	}
	if dealWorkOf(work, won) != nil {
		t.Errorf("the won deal is in the list of open work")
	}
}

func allocate(t *testing.T, e *apptest.AppEnv, deal string, products ...AnyMap) {
	t.Helper()
	var current crmcontracts.DealProductListResponse
	mustCall(t, e, "GET", "/v1/deals/"+deal+"/products", nil, http.StatusOK, &current)
	mustCall(t, e, "PUT", "/v1/deals/"+deal+"/products",
		AnyMap{"expected_tag": current.Tag, "products": products}, http.StatusOK, nil)
}

func readProgress(t *testing.T, e *apptest.AppEnv, query string) crmcontracts.SalesProgress {
	t.Helper()
	var progress crmcontracts.SalesProgress
	mustCall(t, e, "GET", "/v1/analytics/progress?"+query, nil, http.StatusOK, &progress)
	return progress
}

func rosterHolds(roster crmcontracts.SalesGoalRoster, owner string) bool {
	for _, seller := range roster.Data {
		if seller.OwnerId.String() == owner {
			return true
		}
	}
	return false
}

func monthsTotal(months crmcontracts.SalesGoalMonths) int64 {
	var total int64
	for _, month := range months {
		if month != nil {
			total += *month
		}
	}
	return total
}

func dealWorkOf(page crmcontracts.SalesProgressDealPage, deal string) *crmcontracts.SalesProgressDeal {
	for i := range page.Data {
		if page.Data[i].Deal.Id.String() == deal {
			return &page.Data[i]
		}
	}
	return nil
}

func TestASellerReadsTheirOwnPublishedGoalsButCannotChangeThem(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	manager := signedInUser(t, e)
	year := clocktest.Now(t).Year()
	const email, password = "seller@sales-goals.test", "sales-goals-seller-password"
	var user struct {
		ID string `json:"id"`
	}
	mustCall(t, e, http.MethodPost, "/v1/users", AnyMap{"email": email, "display_name": "Sam Seller", "role": "rep"}, http.StatusCreated, &user)
	apptest.SetPasswordByLink(t, e, user.ID, password)
	plan := fmt.Sprintf("/v1/analytics/sales-goals/%s/%d", user.ID, year)

	origin := crmcontracts.SalesGoalDraftInputOrigin("manual")
	draft := crmcontracts.SalesGoalDraftInput{Months: evenMonths(), Reason: "Agreed in the yearly review", Origin: &origin}
	_, saved := planWrite(t, e, "PUT", plan+"/draft", 0, draft)
	status, published := planWrite(t, e, "POST", plan+"/publish", saved.Version,
		crmcontracts.SalesGoalPublishInput{Reason: "Agreed in the yearly review", ConfirmAgreed: true})
	if status != http.StatusOK {
		t.Fatalf("publishing the seller's goals → %d", status)
	}

	if status := e.Call(t, http.MethodPost, "/v1/analytics/targets", monthTarget(user.ID, year), nil, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("a target written over a published plan's month → %d, want 422", status)
	}

	seller := e.AsSecondSeat(t, email, password)
	var own crmcontracts.SalesGoalPlan
	mustCall(t, seller, "GET", plan, nil, http.StatusOK, &own)
	if own.Capabilities.Edit || own.Published == nil || monthsTotal(own.LiveMonths) != yearlyGoal {
		t.Errorf("the seller's own plan = %+v, want the published goals, read-only", own)
	}
	if status, _ := planWrite(t, seller, "PUT", plan+"/draft", published.Version, draft); status < http.StatusBadRequest {
		t.Errorf("the seller saving their own draft → %d, want a refusal", status)
	}
	if status := seller.Call(t, "GET", fmt.Sprintf("/v1/analytics/sales-goals/%s/%d", manager, year), nil, nil, nil); status < http.StatusBadRequest {
		t.Errorf("the seller reading the manager's plan → %d, want a refusal", status)
	}
	var roster crmcontracts.SalesGoalRoster
	mustCall(t, seller, "GET", fmt.Sprintf("/v1/analytics/sales-goals?year=%d", year), nil, http.StatusOK, &roster)
	for _, row := range roster.Data {
		if row.CanEdit {
			t.Errorf("the seller's roster offers to edit %s", row.Name)
		}
	}
	progress := readProgress(t, seller, fmt.Sprintf("period=year&year=%d", year))
	if progress.Capabilities.WriteGoals || progress.Goal.AmountMinor == nil || *progress.Goal.AmountMinor != yearlyGoal {
		t.Errorf("the seller's Progress = goal %+v, write %v; want their published goal, read-only", progress.Goal, progress.Capabilities.WriteGoals)
	}
}
