// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A data question typed in Vietnamese reaches the planner as typed and is
// answered by the reader the Analytics page uses, with the page's numbers.

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

func TestAVietnameseDataQuestionAnswersWithTheAnalyticsNumbers(t *testing.T) {
	lane := &scriptedLane{}
	e := apptest.SetupAppWithOptions(t, compose.WithFilterProposals(lane))
	e.BootstrapWorkspace(t)
	owner := signedInUser(t, e)
	publishGoals(t, e, owner, clocktest.Now(t).Year())
	wonDeal(t, e, 300_000)
	wonDeal(t, e, 450_000)

	// A won count read off a bounded record list would undercount, whichever
	// order the question puts its words in.
	lane.answer(`{"engine":"records","arguments":"{\"plan\":{\"version\":\"v1\",\"target\":\"deal\"}}","count":false,"unsupported_parts":[]}`)
	if status := e.Call(t, http.MethodPost, "/v1/search/data-answer", AnyMap{"question": "tháng này tôi thắng bao nhiêu deal"}, nil, nil); status == http.StatusOK {
		t.Error("a Vietnamese won count was answered from a record list, want the Sales won metric")
	}

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
			query := "period=this_month&target_basis=month&close_window=all_open&scope_kind=owner&scope_id=" + owner + "&metrics=" + test.metric
			mustCall(t, e, http.MethodGet, "/v1/analytics/evaluate?"+query, nil, http.StatusOK, &page)
			answered, shown := got.Evaluation.Metrics[0], page.Metrics[0]
			if !reflect.DeepEqual(answered.Value, shown.Value) || !reflect.DeepEqual(answered.Target, shown.Target) ||
				!reflect.DeepEqual(answered.Coverage.EligibleCount, shown.Coverage.EligibleCount) {
				t.Errorf("the answer reads value %v, target %v, count %v; Analytics shows %v, %v, %v",
					answered.Value, answered.Target, answered.Coverage.EligibleCount, shown.Value, shown.Target, shown.Coverage.EligibleCount)
			}
			if test.count && (answered.Coverage.EligibleCount == nil || *answered.Coverage.EligibleCount != 2) {
				t.Errorf("the won count = %v, want the two deals won this month", answered.Coverage.EligibleCount)
			}
			if !test.count && answered.Target == nil {
				t.Error("the goal question reads no target, want the published monthly goal")
			}
		})
	}
}
