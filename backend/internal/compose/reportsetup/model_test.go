// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reportsetup

import (
	"strings"
	"testing"
)

func setupInput(site string) Input {
	return Input{
		Site: site, Description: "Linden press deals in Germany this quarter", Today: "2026-11-03", Lang: "en",
		Catalog: Catalog{
			Products:   []Option{{ID: "11111111-1111-4111-8111-111111111111", Label: "Linden press line"}},
			Industries: []Option{{ID: "manufacturing", Label: "Manufacturing"}},
			Periods:    []string{"this_month", "this_quarter"},
			Metrics: []MetricOption{
				{ID: "bookings_won", Label: "Sales won", Charts: []string{"bookings_trend", "metric_reading"}},
				{ID: "open_pipeline", Label: "Open pipeline", Charts: []string{"stage_distribution"}},
			},
			Charts:     []Option{{ID: "bookings_trend", Label: "Sales over time"}, {ID: "stage_distribution", Label: "Deals by stage"}, {ID: "metric_reading", Label: "Figure"}},
			Objectives: []Option{{ID: "bookings_won", Label: "Sales won"}},
		},
	}
}

func TestParseResolvesTheCatalogAndNamesBackWhatItCannot(t *testing.T) {
	reply := `{"name":"Linden press in Germany","filters":[{"field":"product","value":"Linden press line","phrase":"Linden press"},{"field":"country","value":"de","phrase":"in Germany"},{"field":"period","value":"this_quarter","phrase":"this quarter"},{"field":"industry","value":"retail","phrase":"retail"}],"metrics":[{"id":"bookings_won","phrase":"sales"},{"id":"churn","phrase":"churn"}],"charts":[{"id":"bookings_trend","phrase":"over time"},{"id":"stage_distribution","phrase":"by stage"}],"objective":null,"clarifications":["Which stages count as open?"],"unsupported":[{"phrase":"by rep tenure","reason":"No field holds a rep's tenure."}]}`
	out, err := Parse(reply, setupInput(SiteCustomReport))
	if err != nil {
		t.Fatal(err)
	}
	if out.Name != "Linden press in Germany" || out.Product != "11111111-1111-4111-8111-111111111111" || out.Country != "DE" || out.Period != "this_quarter" || out.Industry != "" {
		t.Fatalf("resolved %+v, want the product by its label, the country upper-cased and the unknown industry dropped", out)
	}
	if strings.Join(out.Metrics, ",") != "bookings_won" || strings.Join(out.Blocks, ",") != "bookings_trend" {
		t.Fatalf("kept metrics %v and charts %v, want the known metric and the chart it draws", out.Metrics, out.Blocks)
	}
	codes := map[string]int{}
	for _, dropped := range out.Unsupported {
		codes[dropped.Code]++
	}
	if codes[CodeUnknownValue] != 2 || codes[CodeNotOffered] != 1 || codes[CodeNotExpressible] != 1 {
		t.Fatalf("named back %+v, want the industry and the metric as unknown, the undrawn chart as not offered and the model's own phrase", out.Unsupported)
	}
	if len(out.Clarifications) != 1 || len(out.Sources) != 6 {
		t.Fatalf("kept %d clarifications and %d sources: %+v", len(out.Clarifications), len(out.Sources), out.Sources)
	}
}

func TestParseKeepsAnObjectiveOnlyForAnInitiative(t *testing.T) {
	reply := `{"name":"Linden push","filters":[],"metrics":[{"id":"bookings_won","phrase":"sales"}],"charts":[],"objective":{"metric":"bookings_won","target_value":250000,"deadline":"2026-12-31","phrase":"250k by year end"},"clarifications":[],"unsupported":[]}`
	out, err := Parse(reply, setupInput(SiteInitiative))
	if err != nil {
		t.Fatal(err)
	}
	if out.Objective == nil || out.Objective.Metric != "bookings_won" || out.Objective.TargetValue != 250000 || out.Objective.Deadline != "2026-12-31" {
		t.Fatalf("the objective is %+v", out.Objective)
	}
	if len(out.Metrics) != 0 || len(out.Unsupported) != 1 || out.Unsupported[0].Code != CodeNotOffered {
		t.Fatalf("an initiative kept metrics %v, named back %+v", out.Metrics, out.Unsupported)
	}
	report, err := Parse(reply, setupInput(SiteCustomReport))
	if err != nil {
		t.Fatal(err)
	}
	if report.Objective != nil || len(report.Metrics) != 1 {
		t.Fatalf("a custom report kept an objective %+v", report.Objective)
	}
}

func TestParseDropsAPastDeadlineAndAZeroTargetButKeepsTheMetric(t *testing.T) {
	reply := `{"name":"","filters":[],"metrics":[],"charts":[],"objective":{"metric":"Sales won","target_value":0,"deadline":"2026-01-01","phrase":"by January"},"clarifications":[],"unsupported":[]}`
	out, err := Parse(reply, setupInput(SiteInitiative))
	if err != nil {
		t.Fatal(err)
	}
	if out.Objective == nil || out.Objective.Metric != "bookings_won" || out.Objective.TargetValue != 0 || out.Objective.Deadline != "" {
		t.Fatalf("the objective is %+v, want the metric alone", out.Objective)
	}
	if len(out.Unsupported) != 2 {
		t.Fatalf("named back %+v, want the target and the deadline", out.Unsupported)
	}
}

func TestParseRefusesAnEmptyReplyAndAnIdInProse(t *testing.T) {
	if _, err := Parse(`{"name":"","filters":[],"metrics":[],"charts":[],"objective":null,"clarifications":[],"unsupported":[]}`, setupInput(SiteCustomReport)); err == nil {
		t.Fatal("a reply naming nothing was kept")
	}
	out, err := Parse(`{"name":"See 11111111-1111-4111-8111-111111111111","filters":[],"metrics":[],"charts":[],"objective":null,"clarifications":["Is 0198F3A1-7C42-7E0B-9D51-2A6F4B8C1E07 the right deal?","Which quarter?"],"unsupported":[]}`, setupInput(SiteCustomReport))
	if err != nil {
		t.Fatal(err)
	}
	if out.Name != "" || len(out.Clarifications) != 1 {
		t.Fatalf("prose spelling an id was kept: name %q, clarifications %v", out.Name, out.Clarifications)
	}
}
