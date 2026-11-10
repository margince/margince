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

// openGoalFixtureJSON names a goal and leaves its target and deadline unsaid.
const openGoalFixtureJSON = `{
  "description": "More qualified opportunities with manufacturing companies.",
  "today": "2026-11-03", "lang": "en",
  "products": [{"label": "linden", "name": "Linden press line"}],
  "catalog": {
    "industries": [{"id": "manufacturing", "label": "Manufacturing"}],
    "periods": ["this_month", "this_quarter"],
    "metrics": [{"id": "qualified_opportunities_created", "label": "Qualified opportunities", "charts": []}],
    "charts": [],
    "objectives": [{"id": "qualified_opportunities_created", "label": "Qualified opportunities"}]
  }
}`

func runReportSetup(t *testing.T, site, expected string, build func(product func(string) string) string) aitasks.Outcome {
	t.Helper()
	return runReportSetupOn(t, reportSetupFixtureJSON, site, expected, build)
}

func runReportSetupOn(t *testing.T, fixture, site, expected string, build func(product func(string) string) string) aitasks.Outcome {
	t.Helper()
	prepared, err := reportSetupCases{site: site}.Prepare(json.RawMessage(fixture), json.RawMessage(expected))
	if err != nil {
		t.Fatalf("preparing the case: %v", err)
	}
	c, ok := prepared.(*reportSetupCase)
	if !ok {
		t.Fatalf("prepared a %T, want the setup case", prepared)
	}
	request := reportsetup.Request(c.in)
	productRef := func(label string) string {
		for _, product := range c.in.Catalog.Products {
			if product.ID == c.labels[label] {
				return refBeside(t, request, product.Label)
			}
		}
		t.Fatalf("the fixture has no product %q", label)
		return ""
	}
	trace, err := prepared.Run(context.Background(), &replyBrainStub{response: model.Response{Text: build(productRef)}})
	if err != nil {
		t.Fatalf("running the case: %v", err)
	}
	return prepared.Evaluate(trace)
}

func TestTheInitiativeSetupCaseJudgesTheResolvedGoal(t *testing.T) {
	expected := `{"product":"linden","country":"DE","objective":"bookings_won","target_value":250000,"deadline":"2026-12-31","unsupported":0}`
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
			name: "a target the words never gave is a wrong answer",
			build: func(product func(string) string) string {
				return `{"name":"Linden push","filters":[{"field":"product","value":"` + product("linden") + `","phrase":"Linden press line"},{"field":"country","value":"DE","phrase":"in Germany"}],"metrics":[],"charts":[],"objective":{"metric":"bookings_won","target_value":300000,"deadline":"2026-12-31","phrase":"sales won"},"clarifications":[],"unsupported":[]}`
			},
			want: aitasks.OutcomeWrongAnswer,
		},
		{
			name: "the other product is a wrong answer",
			build: func(product func(string) string) string {
				return `{"name":"Tern push","filters":[{"field":"product","value":"` + product("tern") + `","phrase":"Linden"},{"field":"country","value":"DE","phrase":"in Germany"}],"metrics":[],"charts":[],"objective":{"metric":"bookings_won","target_value":250000,"deadline":"2026-12-31","phrase":"sales won"},"clarifications":[],"unsupported":[]}`
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

func TestAnObjectiveWithNoTargetOrDeadlineIsAskedForNotInvented(t *testing.T) {
	expected := `{"industry":"manufacturing","objective":"qualified_opportunities_created","unsupported":0}`
	reply := func(target, deadline string) func(func(string) string) string {
		return func(func(string) string) string {
			return `{"name":"Manufacturing pipeline","filters":[{"field":"industry","value":"manufacturing","phrase":"manufacturing companies"}],"metrics":[],"charts":[],"objective":{"metric":"qualified_opportunities_created","target_value":` + target + `,"deadline":` + deadline + `,"phrase":"more qualified opportunities"},"clarifications":["What target and deadline should the initiative have?"],"unsupported":[]}`
		}
	}
	if outcome := runReportSetupOn(t, openGoalFixtureJSON, reportsetup.SiteInitiative, expected, reply("null", "null")); outcome.Result != aitasks.OutcomeAccepted {
		t.Fatalf("a null target and deadline were %s (%s), want accepted", outcome.Result, outcome.Detail)
	}
	if outcome := runReportSetupOn(t, openGoalFixtureJSON, reportsetup.SiteInitiative, expected, reply("null", `"2026-12-31"`)); outcome.Result != aitasks.OutcomeWrongAnswer {
		t.Fatalf("a deadline the words never gave was %s (%s), want a wrong answer", outcome.Result, outcome.Detail)
	}
}

func TestTheCustomReportSetupCaseJudgesMetricsAndWhatIsNamedBack(t *testing.T) {
	expected := `{"product":"linden","period":"this_quarter","metrics":["bookings_won","open_pipeline"],"charts":["bookings_trend","stage_distribution"],"unsupported":1}`
	accepted := runReportSetup(t, reportsetup.SiteCustomReport, expected, func(product func(string) string) string {
		return `{"name":"Linden this quarter","filters":[{"field":"product","value":"` + product("linden") + `","phrase":"Linden"},{"field":"period","value":"this_quarter","phrase":"this quarter"}],"metrics":[{"id":"bookings_won","phrase":"sales won"},{"id":"open_pipeline","phrase":"open pipeline"}],"charts":[{"id":"bookings_trend","phrase":"over time"}],"objective":null,"clarifications":[],"unsupported":[{"phrase":"discount per rep","reason":"No metric holds a discount."}]}`
	})
	if accepted.Result != aitasks.OutcomeAccepted {
		t.Fatalf("outcome %s (%s)", accepted.Result, accepted.Detail)
	}
	mapped := runReportSetup(t, reportsetup.SiteCustomReport, expected, func(product func(string) string) string {
		return `{"name":"Linden this quarter","filters":[{"field":"product","value":"` + product("linden") + `","phrase":"Linden"},{"field":"period","value":"this_quarter","phrase":"this quarter"}],"metrics":[{"id":"bookings_won","phrase":"sales won"},{"id":"open_pipeline","phrase":"open pipeline"}],"charts":[{"id":"bookings_trend","phrase":"over time"}],"objective":null,"clarifications":[],"unsupported":[]}`
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
