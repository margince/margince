// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A data question typed in Vietnamese reaches the planner as typed. The reader
// the Analytics page uses answers it, with the page's numbers for the same
// workspace scope. searchplan's admission tests own the refusals.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

func TestAVietnameseDataQuestionAnswersWithTheWorkspaceAnalyticsNumbers(t *testing.T) {
	t.Parallel()
	lane := &scriptedLane{}
	e := apptest.SetupAppWithOptions(t, compose.WithFilterProposals(lane))
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	owner := signedInUser(t, e)
	publishGoals(t, e, owner, clocktest.Now(t).Year())
	wonDeal(t, e, 300_000)
	wonDeal(t, e, 450_000)
	// A colleague's win tells the workspace figure from the owner's.
	var colleague struct {
		ID string `json:"id"`
	}
	mustCall(t, e, http.MethodPost, "/v1/users", AnyMap{"email": "colleague@vi-search.test", "display_name": "Lan Colleague", "role": "rep"}, http.StatusCreated, &colleague)
	apptest.SetPasswordByLink(t, e, colleague.ID, "vi-search-colleague-password")
	publishGoals(t, e, colleague.ID, clocktest.Now(t).Year())
	stages := apptest.DiscoverSeededPipeline(t, e)
	theirs := createdID(t, e, "/v1/deals", AnyMap{
		"name": "Won by a colleague", "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "owner_id": colleague.ID,
		"source": "manual", "amount_minor": 200_000, "currency": "EUR",
	})
	mustCall(t, e, http.MethodPost, "/v1/deals/"+theirs+"/advance",
		AnyMap{"to_stage_id": stages.Won, "won_without_contract_reason": "verbal"}, http.StatusOK, nil)

	for _, test := range []struct {
		question, metric string
		count            bool
	}{
		{"tháng này tôi thắng bao nhiêu deal", "bookings_won", true},
		{"Cuối tháng có đạt mục tiêu không?", "bookings_won", false},
	} {
		t.Run(test.question, func(t *testing.T) {
			// The planner names the workspace: a user's id reaches it only as a ref.
			selection := fmt.Sprintf(`{"scope":{"kind":"workspace"},"period":"this_month","target_basis":"month","close_window":"all_open","metrics":[%q],"blocks":[]}`, test.metric)
			plan, err := json.Marshal(map[string]any{"engine": "reporting", "arguments": selection, "count": test.count, "unsupported_parts": []string{}})
			if err != nil {
				t.Fatal(err)
			}
			lane.answer(string(plan))
			var got crmcontracts.SearchDataAnswer
			mustCall(t, e, http.MethodPost, "/v1/search/data-answer", AnyMap{"question": test.question}, http.StatusOK, &got)
			if !strings.Contains(lane.asked[len(lane.asked)-1], test.question) {
				t.Errorf("the planner was not asked %q as typed", test.question)
			}
			if got.Outcome != crmcontracts.SearchDataAnswerOutcomeAnswered || got.Evaluation == nil || len(got.Evaluation.Metrics) != 1 {
				t.Fatalf("the answer = %+v, want one metric from the data reader", got)
			}

			var page crmcontracts.ReportingEvaluation
			query := "period=this_month&target_basis=month&close_window=all_open&scope_kind=workspace&metrics=" + test.metric
			mustCall(t, e, http.MethodGet, "/v1/analytics/evaluate?"+query, nil, http.StatusOK, &page)
			answered, shown := got.Evaluation.Metrics[0], page.Metrics[0]
			if !reflect.DeepEqual(answered.Value, shown.Value) || !reflect.DeepEqual(answered.Target, shown.Target) ||
				!reflect.DeepEqual(answered.Coverage.EligibleCount, shown.Coverage.EligibleCount) {
				t.Errorf("the answer reads value %v, target %v, count %v; Analytics shows %v, %v, %v",
					held(answered.Value), held(answered.Target), held(answered.Coverage.EligibleCount), held(shown.Value), held(shown.Target), held(shown.Coverage.EligibleCount))
			}
			if test.count && (answered.Coverage.EligibleCount == nil || *answered.Coverage.EligibleCount != 3) {
				t.Errorf("the won count = %v, want the three deals the workspace won this month", held(answered.Coverage.EligibleCount))
			}
			if !test.count && answered.Target == nil {
				t.Error("the goal question reads no target, want the published monthly goal")
			}
		})
	}
}

// held prints what a pointer holds, so a failure shows the figure.
func held[T float64 | int64](figure *T) string {
	if figure == nil {
		return "none"
	}
	return fmt.Sprint(*figure)
}
