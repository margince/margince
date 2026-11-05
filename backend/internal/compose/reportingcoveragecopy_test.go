// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestReportingCoverageReadersUseTheirSavedLanguage(t *testing.T) {
	pipeline := ids.NewV7()
	for _, language := range []string{"en", "de", "vi"} {
		frame := crmcontracts.ReportingContext{ProseLanguage: &language, PipelineId: ptrUUID(pipeline)}
		for _, tc := range []struct {
			read   metricReader
			status crmcontracts.ReportingStatus
			reason string
		}{
			{readReportingQualified, "not_configured", reporting.ExplanationCopy(frame).Say(reportingCoverageWords.Qualification)},
			{readReportingMeetings, "unsupported", reporting.ExplanationCopy(frame).Say(reportingCoverageWords.MeetingPipeline)},
			{readReportingAccepted, "unsupported", reporting.ExplanationCopy(frame).Say(reportingCoverageWords.AcceptancePipeline)},
		} {
			facts, coverage, err := tc.read(t.Context(), nil, frame, crmcontracts.ReportingFramework{}, metricEvaluator{})
			if err != nil {
				t.Fatal(err)
			}
			if len(facts) != 0 || coverage.Status != tc.status || coverage.Reason == nil || *coverage.Reason != tc.reason {
				t.Fatalf("%s changed coverage: %+v", language, coverage)
			}
		}
	}
}
