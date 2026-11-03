// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/reportsetup"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const reportSetupFixtureJSON = `{
  "description": "Push the Linden press line in Germany: 250000 EUR of sales won by the end of December.",
  "today": "2026-11-03", "lang": "en",
  "products": [{"label": "linden", "name": "Linden press line"}, {"label": "tern", "name": "Tern portal"}],
  "catalog": {
    "industries": [{"id": "manufacturing", "label": "Manufacturing"}],
    "periods": ["this_month", "this_quarter"],
    "metrics": [{"id": "bookings_won", "label": "Sales won", "charts": ["bookings_trend"]}, {"id": "open_pipeline", "label": "Open pipeline", "charts": ["stage_distribution"]}],
    "charts": [{"id": "bookings_trend", "label": "bookings_trend"}, {"id": "stage_distribution", "label": "stage_distribution"}],
    "objectives": [{"id": "bookings_won", "label": "Sales won"}]
  }
}`

func runReportSetup(t *testing.T, site, expected string, build func(product func(string) string) string) aitasks.Outcome {
	t.Helper()
	prepared, err := reportSetupCases{site: site}.Prepare(json.RawMessage(reportSetupFixtureJSON), json.RawMessage(expected))
	if err != nil {
		t.Fatalf("preparing the case: %v", err)
	}
	c, ok := prepared.(*reportSetupCase)
	if !ok {
		t.Fatalf("prepared a %T, want the setup case", prepared)
	}
	trace, err := prepared.Run(context.Background(), &replyBrainStub{response: model.Response{Text: build(func(label string) string { return c.labels[label] })}})
	if err != nil {
		t.Fatalf("running the case: %v", err)
	}
	return prepared.Evaluate(trace)
}

func TestTheInitiativeSetupCaseJudgesTheResolvedGoal(t *testing.T) {
	expected := `{"product":"linden","country":"DE","objective":"bookings_won","unsupported":0}`
	for _, tc := range []struct {
		name  string
		build func(product func(string) string) string
		want  string
	}{
		{
			name: "the goal as written",
			build: func(product func(string) string) string {
				return `{"name":"Linden push","filters":[{"field":"product","value":"` + product("linden") + `","phrase":"Linden press line"},{"field":"country","value":"DE","phrase":"in Germany"}],"metrics":[],"charts":[],"objective":{"metric":"bookings_won","target_value":250000,"deadline":"2026-12-31","phrase":"250000 EUR of sales won by the end of December"},"clarifications":[],"unsupported":[]}`
			},
			want: aitasks.OutcomeAccepted,
		},
		{
			name: "the other product is a wrong answer",
			build: func(product func(string) string) string {
				return `{"name":"Tern push","filters":[{"field":"product","value":"` + product("tern") + `","phrase":"Linden"},{"field":"country","value":"DE","phrase":"in Germany"}],"metrics":[],"charts":[],"objective":{"metric":"bookings_won","phrase":"sales won"},"clarifications":[],"unsupported":[]}`
			},
			want: aitasks.OutcomeWrongAnswer,
		},
		{
			name: "an invented product id is dropped, which leaves the product unresolved",
			build: func(product func(string) string) string {
				return `{"name":"Linden push","filters":[{"field":"product","value":"01a19fad-0000-7000-8000-000000000000","phrase":"Linden"},{"field":"country","value":"DE","phrase":"in Germany"}],"metrics":[],"charts":[],"objective":{"metric":"bookings_won","phrase":"sales won"},"clarifications":[],"unsupported":[]}`
			},
			want: aitasks.OutcomeWrongAnswer,
		},
		{
			name:  "a reply that is not the shape asked for is invalid",
			build: func(product func(string) string) string { return `Linden in Germany.` },
			want:  aitasks.OutcomeInvalid,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			outcome := runReportSetup(t, reportsetup.SiteInitiative, expected, tc.build)
			if outcome.Result != tc.want {
				t.Fatalf("outcome %s (%s), want %s", outcome.Result, outcome.Detail, tc.want)
			}
		})
	}
}

func TestTheCustomReportSetupCaseJudgesMetricsAndWhatIsNamedBack(t *testing.T) {
	expected := `{"product":"linden","period":"this_quarter","metrics":["bookings_won","open_pipeline"],"unsupported":1}`
	accepted := runReportSetup(t, reportsetup.SiteCustomReport, expected, func(product func(string) string) string {
		return `{"name":"Linden this quarter","filters":[{"field":"product","value":"` + product("linden") + `","phrase":"Linden"},{"field":"period","value":"this_quarter","phrase":"this quarter"}],"metrics":[{"id":"bookings_won","phrase":"sales won"},{"id":"open_pipeline","phrase":"open pipeline"}],"charts":[{"id":"bookings_trend","phrase":"over time"}],"objective":null,"clarifications":[],"unsupported":[{"phrase":"discount per rep","reason":"No metric holds a discount."}]}`
	})
	if accepted.Result != aitasks.OutcomeAccepted {
		t.Fatalf("outcome %s (%s)", accepted.Result, accepted.Detail)
	}
	mapped := runReportSetup(t, reportsetup.SiteCustomReport, expected, func(product func(string) string) string {
		return `{"name":"Linden this quarter","filters":[{"field":"product","value":"` + product("linden") + `","phrase":"Linden"},{"field":"period","value":"this_quarter","phrase":"this quarter"}],"metrics":[{"id":"bookings_won","phrase":"sales won"},{"id":"open_pipeline","phrase":"open pipeline"}],"charts":[],"objective":null,"clarifications":[],"unsupported":[]}`
	})
	if mapped.Result != aitasks.OutcomeWrongAnswer || !strings.Contains(mapped.Detail, "named back") {
		t.Fatalf("a reply naming nothing back was %s (%s)", mapped.Result, mapped.Detail)
	}
}

func TestTheReportSetupCaseRefusesAnExpectationTheDrawerCannotTake(t *testing.T) {
	if _, err := (reportSetupCases{site: reportsetup.SiteInitiative}).Prepare(json.RawMessage(reportSetupFixtureJSON), json.RawMessage(`{"metrics":["bookings_won"]}`)); err == nil {
		t.Fatal("an initiative expectation with metrics was prepared")
	}
	if _, err := (reportSetupCases{site: reportsetup.SiteCustomReport}).Prepare(json.RawMessage(reportSetupFixtureJSON), json.RawMessage(`{"objective":"bookings_won"}`)); err == nil {
		t.Fatal("a custom report expectation with an objective was prepared")
	}
	if _, err := (reportSetupCases{site: reportsetup.SiteCustomReport}).Prepare(json.RawMessage(reportSetupFixtureJSON), json.RawMessage(`{"product":"globex"}`)); err == nil || !strings.Contains(err.Error(), "globex") {
		t.Fatalf("an expected product outside the fixture was prepared: %v", err)
	}
}
