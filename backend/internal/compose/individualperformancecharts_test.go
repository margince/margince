// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// withheldOwnerLeaks projects every metric of the catalog in every block it
// offers. The facts are owned by the reader and by a colleague they may not
// measure. It names each chart with a point carrying the colleague.
func withheldOwnerLeaks(project func(*reporting.Evaluation) error) ([]string, error) {
	reader, colleague := ids.NewV7(), ids.NewV7()
	value := 100.0
	at := time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)
	var leaks []string
	for _, spec := range reportingMetrics {
		for _, block := range spec.definition.Blocks {
			out := reporting.Evaluation{Result: crmcontracts.ReportingEvaluation{
				Context: crmcontracts.ReportingContext{
					Timezone: "UTC", StateAt: at, EvaluatedAt: at.AddDate(0, 0, 30),
					Interval: crmcontracts.ReportingWindow{StartAt: at.AddDate(0, 0, -7), EndAt: at.AddDate(0, 0, 21)},
				},
				Selection: crmcontracts.ReportingSelection{Metrics: []crmcontracts.ReportingMetricID{spec.definition.Id}, Blocks: []crmcontracts.ReportingBlockKind{block}},
				Metrics: []crmcontracts.ReportingMetric{{
					Id: spec.definition.Id, Unit: spec.definition.Unit, Coverage: crmcontracts.ReportingCoverage{Status: "ok"},
					Evidence: crmcontracts.ReportingEvidenceRef{Metric: spec.definition.Id, ContextId: reportingInterval},
				}},
			}}
			for _, contextID := range []string{reportingInterval, reportingState} {
				for _, owner := range []ids.UUID{reader, colleague} {
					out.Facts = append(out.Facts, reporting.Fact{
						Metric: spec.definition.Id, ContextID: contextID, OwnerID: owner, OwnerLabel: "seat " + owner.String(),
						OwnerWithheld: owner == colleague, SourceID: ids.NewV7(), PlacedAt: &at,
						Row: crmcontracts.ReportingEvidenceRow{Key: owner.String(), Value: &value, OccurredAt: &at},
					})
				}
			}
			if err := project(&out); err != nil {
				return nil, err
			}
			for _, chart := range out.Result.Charts {
				if slices.ContainsFunc(chart.Points, func(point crmcontracts.ReportingPoint) bool {
					return strings.Contains(point.Key+point.Label, colleague.String())
				}) {
					leaks = append(leaks, string(spec.definition.Id)+" in "+string(block))
				}
			}
		}
	}
	return leaks, nil
}

// Every chart the metric catalog can draw keeps a withheld owner out of its
// points. A new metric or block that groups by owner some other way fails here.
func TestNoReportingChartDrawsAWithheldOwner(t *testing.T) {
	leaks, err := withheldOwnerLeaks(projectReportingCharts)
	if err != nil {
		t.Fatalf("projecting the catalog: %v", err)
	}
	if len(leaks) > 0 {
		t.Fatalf("these charts draw an owner the reader may not measure:\n%s", strings.Join(leaks, "\n"))
	}
}

// A projection that groups by owner and ignores the withheld mark must be
// caught, or the test above proves nothing.
func TestTheChartCensusCatchesAProjectionIgnoringTheMark(t *testing.T) {
	byEveryOwner := func(out *reporting.Evaluation) error {
		chart := crmcontracts.ReportingChart{Kind: reportingOwnerAttainment}
		for _, fact := range out.Facts {
			chart.Points = append(chart.Points, crmcontracts.ReportingPoint{Key: fact.OwnerID.String(), Label: fact.OwnerLabel})
		}
		out.Result.Charts = append(out.Result.Charts, chart)
		return nil
	}
	leaks, err := withheldOwnerLeaks(byEveryOwner)
	if err != nil {
		t.Fatalf("projecting the catalog: %v", err)
	}
	if len(leaks) == 0 {
		t.Fatal("a projection drawing every owner passed the census")
	}
}
