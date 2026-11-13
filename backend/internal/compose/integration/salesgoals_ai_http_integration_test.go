// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The two model-written parts of sales goals over the wire: the summary of a
// Progress reading and the proposed months of a seller's plan.

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// scriptedLane answers every request with its current reply and keeps what
// it was asked.
type scriptedLane struct {
	mu    sync.Mutex
	reply string
	asked []string
}

func (l *scriptedLane) Complete(_ context.Context, req model.Request) (model.Response, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.asked = append(l.asked, req.Messages[0].Content)
	return model.Response{Text: l.reply, ServedModel: "scripted"}, nil
}

func (l *scriptedLane) answer(reply string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.reply = reply
}

func (l *scriptedLane) calls() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.asked)
}

// wonDeal wins a deal of amount for the signed-in user and returns its id.
func wonDeal(t *testing.T, e *apptest.AppEnv, amount int64) string {
	t.Helper()
	stages := apptest.DiscoverSeededPipeline(t, e)
	deal := createdID(t, e, "/v1/deals", AnyMap{
		"name": "Won for the goals", "pipeline_id": stages.PipelineID, "stage_id": stages.Open,
		"source": "manual", "amount_minor": amount, "currency": "EUR",
	})
	mustCall(t, e, "POST", "/v1/deals/"+deal+"/advance",
		AnyMap{"to_stage_id": stages.Won, "won_without_contract_reason": "verbal"}, http.StatusOK, nil)
	return deal
}

const progressSummaryReply = `{"paragraphs":[[{"text":"You've closed {{progress:closed}} of the {{progress:goal}} goal.","kind":"observation","cells":["progress:closed","progress:goal"]}]],"action_ids":[],"next_decision":null,"confidence":"medium"}`

func TestProgressIsSummedUpFromItsOwnFiguresAndKeptPerReader(t *testing.T) {
	lane := &scriptedLane{reply: progressSummaryReply}
	e := apptest.SetupAppWithOptions(t, compose.WithSalesAssessment(lane, "test-routing"))
	e.BootstrapWorkspace(t)
	owner := signedInUser(t, e)
	year := clocktest.Now(t).Year()
	publishGoals(t, e, owner, year)
	wonDeal(t, e, 300_000)
	ask := AnyMap{"period": "year", "year": year}

	var summary crmcontracts.SalesAssessment
	mustCall(t, e, "POST", "/v1/analytics/progress/summary", ask, http.StatusOK, &summary)
	if summary.GeneratedBy != crmcontracts.WrittenByModel || len(summary.Sentences) != 1 || summary.NextDecision != nil {
		t.Fatalf("the summary = %+v, want the one written sentence and no decision", summary)
	}
	shown := map[string]string{}
	for _, cell := range summary.Cells {
		shown[cell.Id] = cell.Value
	}
	if shown["progress:closed"] != "3000.00 EUR" || shown["progress:goal"] != "1200000.00 EUR" || shown["progress:missing"] != "1197000.00 EUR" {
		t.Errorf("the summary read %v, want the page's closed revenue, goal and what is missing", shown)
	}
	if summary.EvaluationKey == "" {
		t.Error("the summary names no evaluation it was written from")
	}

	mustCall(t, e, "POST", "/v1/analytics/progress/summary", ask, http.StatusOK, &summary)
	if lane.calls() != 1 {
		t.Errorf("the same figures asked the model %d times, want the kept summary", lane.calls())
	}
	lane.answer(`{"paragraphs":[[{"text":"You've closed 9999 EUR.","kind":"observation","cells":["progress:closed"]}]],"action_ids":[],"next_decision":null,"confidence":"high"}`)
	mustCall(t, e, "POST", "/v1/analytics/progress/summary", AnyMap{"period": "year", "year": year, "refresh": true}, http.StatusOK, &summary)
	if lane.calls() < 2 || summary.GeneratedBy != crmcontracts.WrittenByDeterministic || len(summary.Sentences) != 0 {
		t.Errorf("a refreshed summary quoting a figure the page does not hold = %+v after %d calls, want no sentences", summary, lane.calls())
	}

	for name, body := range map[string]AnyMap{
		"an unknown period":       {"period": "week", "year": year},
		"a month with no month":   {"period": "month", "year": year},
		"a scope kind with no id": {"period": "year", "year": year, "scope_kind": "owner"},
	} {
		if status := e.Call(t, "POST", "/v1/analytics/progress/summary", body, nil, nil); status != http.StatusUnprocessableEntity {
			t.Errorf("%s → %d, want 422", name, status)
		}
	}
}

func TestProgressWithNoModelAnswersItsFiguresAndNoSentences(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	wonDeal(t, e, 300_000)
	var summary crmcontracts.SalesAssessment
	mustCall(t, e, "POST", "/v1/analytics/progress/summary", AnyMap{"period": "year", "year": clocktest.Now(t).Year()}, http.StatusOK, &summary)
	if summary.GeneratedBy != crmcontracts.WrittenByDeterministic || len(summary.Sentences) != 0 || len(summary.Cells) == 0 {
		t.Fatalf("the summary without a model = %+v, want the figures and no sentence", summary)
	}
}

func TestASellerIsSummedUpOnlyWithinTheirOwnProgress(t *testing.T) {
	lane := &scriptedLane{reply: progressSummaryReply}
	e := apptest.SetupAppWithOptions(t, compose.WithSalesAssessment(lane, "test-routing"))
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	manager := signedInUser(t, e)
	const email, password = "seller@progress-summary.test", "progress-summary-seller-password"
	var user struct {
		ID string `json:"id"`
	}
	mustCall(t, e, http.MethodPost, "/v1/users", AnyMap{"email": email, "display_name": "Sam Seller", "role": "rep"}, http.StatusCreated, &user)
	apptest.SetPasswordByLink(t, e, user.ID, password)
	seller := e.AsSecondSeat(t, email, password)
	year := clocktest.Now(t).Year()
	theirs := AnyMap{"period": "year", "year": year, "scope_kind": "owner", "scope_id": manager}
	if status := seller.Call(t, "POST", "/v1/analytics/progress/summary", theirs, nil, nil); status != http.StatusNotFound {
		t.Errorf("a seller asking for a colleague's summary → %d, want 404", status)
	}
	if lane.calls() != 0 {
		t.Errorf("a refused summary asked the model %d times", lane.calls())
	}
}

const proposedMonthsReply = `{"months":[100,100,100,100,100,100,100,100,300,300,100,100],"reason":"Autumn carries the year."}`

func TestAManagerIsProposedASellersMonthsFromTheirClosedRevenue(t *testing.T) {
	lane := &scriptedLane{reply: proposedMonthsReply}
	e := apptest.SetupAppWithOptions(t, compose.WithReportSetup(lane))
	e.BootstrapWorkspace(t)
	owner := signedInUser(t, e)
	// A month that has ended whatever today is, in the year before the plan's.
	closed := clocktest.Now(t).UTC().AddDate(0, -2, 0)
	closed = time.Date(closed.Year(), closed.Month(), 15, 12, 0, 0, 0, time.UTC)
	if _, err := e.Owner.Exec(t.Context(), `UPDATE deal SET closed_at = $2 WHERE id = $1`, wonDeal(t, e, 300_000), closed); err != nil {
		t.Fatalf("moving the deal's close into an ended month: %v", err)
	}
	plan := fmt.Sprintf("/v1/analytics/sales-goals/%s/%d", owner, closed.Year()+1)

	var proposal crmcontracts.SalesGoalProposal
	mustCall(t, e, "POST", plan+"/proposed-months", AnyMap{}, http.StatusOK, &proposal)
	if len(proposal.Months) != 12 || proposal.Months[0] != 100_00 || proposal.Months[8] != 300_00 || proposal.TotalMinor != 1600_00 {
		t.Fatalf("the proposal = %+v, want the model's months in minor units", proposal)
	}
	if proposal.GeneratedBy != crmcontracts.WrittenByModel || proposal.Reason != "Autumn carries the year." || proposal.ModelUsed == nil {
		t.Errorf("the proposal is marked %q by %v with reason %q", proposal.GeneratedBy, proposal.ModelUsed, proposal.Reason)
	}
	// A month that has ended reads as a measured zero, a later one as unknown.
	today := clocktest.Now(t).UTC()
	months := make([]string, 12)
	for i := range months {
		months[i] = "null"
		if closed.Year() < today.Year() || time.Month(i+1) < today.Month() {
			months[i] = "0"
		}
	}
	months[closed.Month()-1] = "3000"
	if want := fmt.Sprintf(`{"year":%d,"months":[%s]}`, closed.Year(), strings.Join(months, ",")); !strings.Contains(lane.asked[0], want) {
		t.Errorf("the model read %s, want the won deal in its own month as %s", lane.asked[0], want)
	}

	mustCall(t, e, "POST", plan+"/proposed-months", AnyMap{"yearly_total_minor": 1_000_001}, http.StatusOK, &proposal)
	if proposal.TotalMinor != 1_000_001 || monthsSum(proposal.Months) != 1_000_001 || !strings.Contains(lane.asked[1], `"yearly_goal":10000`) {
		t.Errorf("with a yearly goal the months add up to %d, want it to the minor unit", proposal.TotalMinor)
	}
	var untouched crmcontracts.SalesGoalPlan
	mustCall(t, e, "GET", plan, nil, http.StatusOK, &untouched)
	if untouched.Version != 0 || untouched.Draft != nil {
		t.Errorf("a proposal wrote the plan: %+v", untouched)
	}

	lane.answer(`{"months":[1,2,3],"reason":"Short."}`)
	if status := e.Call(t, "POST", plan+"/proposed-months", AnyMap{}, nil, nil); status != http.StatusServiceUnavailable {
		t.Errorf("a reply that is no plan → %d, want 503", status)
	}
	asked := lane.calls()
	var problem crmcontracts.Problem
	empty := fmt.Sprintf("/v1/analytics/sales-goals/%s/%d/proposed-months", owner, closed.Year()+3)
	if status := e.Call(t, "POST", empty, AnyMap{}, nil, &problem); status != http.StatusConflict || problem.Code != "no_sales_history" {
		t.Errorf("a year with no closed revenue before it → %d %+v, want 409 no_sales_history", status, problem)
	}
	if status := e.Call(t, "POST", plan+"/proposed-months", AnyMap{"yearly_total_minor": 0}, nil, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("a yearly goal of zero → %d, want 422", status)
	}
	if lane.calls() != asked {
		t.Errorf("a refused proposal asked the model")
	}
}

// The months are shaped by what the seller won. While individual performance
// is off a colleague's plan is out of reach, so no model is asked.
func TestAColleaguesMonthsAreNotProposedWhileIndividualPerformanceIsOff(t *testing.T) {
	lane := &scriptedLane{reply: proposedMonthsReply}
	e := apptest.SetupAppWithOptions(t, compose.WithReportSetup(lane))
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	var colleague struct {
		ID string `json:"id"`
	}
	mustCall(t, e, http.MethodPost, "/v1/users", AnyMap{"email": "seller@goal-switch.test", "display_name": "Sam Seller", "role": "rep"}, http.StatusCreated, &colleague)
	path := fmt.Sprintf("/v1/analytics/sales-goals/%s/%d/proposed-months", colleague.ID, clocktest.Now(t).Year())
	if status := e.Call(t, http.MethodPost, path, AnyMap{}, nil, nil); status != http.StatusConflict {
		t.Fatalf("POST %s with the switch on = %d, want 409 for a seller with no sales yet", path, status)
	}
	mustCall(t, e, http.MethodPatch, "/v1/installation/settings", AnyMap{individualPerformanceKey: false}, http.StatusOK, nil)
	if status := e.Call(t, http.MethodPost, path, AnyMap{}, nil, nil); status != http.StatusNotFound {
		t.Errorf("POST %s with the switch off = %d, want 404", path, status)
	}
	if lane.calls() != 0 {
		t.Errorf("a refused proposal asked the model %d times", lane.calls())
	}
}

func monthsSum(months []int64) int64 {
	var total int64
	for _, month := range months {
		total += month
	}
	return total
}

func TestOnlyWhoMaySetASellersGoalsIsProposedThem(t *testing.T) {
	lane := &scriptedLane{reply: proposedMonthsReply}
	e := apptest.SetupAppWithOptions(t, compose.WithReportSetup(lane))
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	manager := signedInUser(t, e)
	year := clocktest.Now(t).Year()
	const email, password = "seller@goal-proposal.test", "goal-proposal-seller-password"
	var user struct {
		ID string `json:"id"`
	}
	mustCall(t, e, http.MethodPost, "/v1/users", AnyMap{"email": email, "display_name": "Sam Seller", "role": "rep"}, http.StatusCreated, &user)
	apptest.SetPasswordByLink(t, e, user.ID, password)
	seller := e.AsSecondSeat(t, email, password)
	own := fmt.Sprintf("/v1/analytics/sales-goals/%s/%d/proposed-months", user.ID, year)
	if status := seller.Call(t, "POST", own, AnyMap{}, nil, nil); status != http.StatusForbidden {
		t.Errorf("a seller asking for their own proposal → %d, want 403", status)
	}
	theirs := fmt.Sprintf("/v1/analytics/sales-goals/%s/%d/proposed-months", manager, year)
	if status := seller.Call(t, "POST", theirs, AnyMap{}, nil, nil); status != http.StatusNotFound {
		t.Errorf("a seller asking for a colleague's proposal → %d, want 404", status)
	}
	if lane.calls() != 0 {
		t.Errorf("a refused proposal asked the model %d times", lane.calls())
	}

	bare := apptest.SetupApp(t)
	bare.BootstrapWorkspace(t)
	var problem crmcontracts.Problem
	path := fmt.Sprintf("/v1/analytics/sales-goals/%s/%d/proposed-months", signedInUser(t, bare), year)
	if status := bare.Call(t, "POST", path, AnyMap{}, nil, &problem); status != http.StatusConflict || problem.Code != "ai_not_configured" {
		t.Errorf("a proposal with no model → %d %+v, want 409 ai_not_configured", status, problem)
	}
}
