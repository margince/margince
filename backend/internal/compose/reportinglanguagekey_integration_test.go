// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/settings"
)

func TestReportingEvidenceAndExportSurviveALanguageChange(t *testing.T) {
	f := reportingBusiness(t)
	config := settings.New(f.env.Pool, settings.NewRegistry(identity.BaseLanguage))
	selection := f.selection()
	selection.PipelineId = nil
	selection.Metrics = []crmcontracts.ReportingMetricID{"meetings_held", "open_pipeline"}
	selection.Blocks = []crmcontracts.ReportingBlockKind{"stage_distribution"}
	if err := settings.Set(f.human, config, identity.BaseLanguage, "de"); err != nil {
		t.Fatal(err)
	}
	original, err := f.service.Evaluate(f.human, selection)
	if err != nil {
		t.Fatal(err)
	}
	expectation := reporting.EvidenceExpectation{Key: original.EvaluationKey, At: original.Context.EvaluatedAt, FrameworkRevision: original.Context.FrameworkRevision}
	for _, language := range []string{"en", "vi"} {
		if err := settings.Set(f.human, config, identity.BaseLanguage, language); err != nil {
			t.Fatal(err)
		}
		receipt, err := f.service.EvaluatedReceipt(f.human, selection, expectation)
		if err != nil {
			t.Fatal(err)
		}
		if receipt.EvaluationKey != original.EvaluationKey || receipt.Context.ProseLanguage == nil || *receipt.Context.ProseLanguage != language {
			t.Fatalf("wrong receipt: %+v", receipt)
		}
		evidence, err := f.service.Evidence(f.human, selection, "meetings_held", "interval", "", nil, nil, 50, expectation)
		if err != nil {
			t.Fatal(err)
		}
		if len(evidence.Rows) != 12 {
			t.Fatalf("evidence changed: %+v", evidence)
		}
	}
	if original.Context.ProseLanguage == nil || *original.Context.ProseLanguage != "de" {
		t.Fatal("hashing mutated the response")
	}
}
