// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

func TestFrozenReportingKeepsItsExplanationLanguage(t *testing.T) {
	langcopytest.Census(t, reportingWords)
	langcopytest.Census(t, reportingLabelWords)
	langcopytest.Census(t, reportingForecastLabels)
	langcopytest.Census(t, reportingForecastWords)
	langcopytest.Census(t, reportingForecastCaveats)
	langcopytest.Census(t, reportingCoverageWords)
	for _, language := range []string{"", "en", "de", "vi"} {
		frame := crmcontracts.ReportingContext{Timezone: "UTC"}
		if language != "" {
			frame.ProseLanguage = &language
		}
		before := crmcontracts.ReportingEvaluation{Context: frame, Metrics: []crmcontracts.ReportingMetric{{Id: reportingForecastLanding, Coverage: crmcontracts.ReportingCoverage{Status: "ok"}}}}
		raw, err := json.Marshal(before)
		if err != nil {
			t.Fatal(err)
		}
		var stored crmcontracts.ReportingEvaluation
		if err := json.Unmarshal(raw, &stored); err != nil {
			t.Fatal(err)
		}
		evaluator := metricEvaluator{}
		projected, err := evaluator.ProjectFrozen(stored, nil)
		if err != nil {
			t.Fatal(err)
		}
		coverage := projected.Metrics[0].Coverage
		if coverage.Reason == nil || *coverage.Reason != reporting.ExplanationCopy(frame).Say(reportingWords.RestrictedHistory) || !coverage.Withheld || projected.Metrics[0].Value != nil {
			t.Fatalf("%s changed frozen language or access: %+v", language, projected)
		}
	}
}
