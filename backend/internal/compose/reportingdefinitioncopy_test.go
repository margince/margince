// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"reflect"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestReportingCatalogTranslationsCoverExactlyTheRegisteredMetrics(t *testing.T) {
	tables := map[string]map[crmcontracts.ReportingMetricID]langcopy.Phrase{
		"definition":  reportingDefinitionWords,
		"attribution": reportingAttributionWords,
		"incomplete":  reportingIncompleteWords,
	}
	langcopytest.Census(t, reportingBreakdownLabels)
	langcopytest.Census(t, reportingLeadStatusLabels)
	grants := map[string]principal.ObjectGrant{}
	for name, table := range tables {
		langcopytest.Census(t, table)
		if len(table) != len(reportingMetrics) {
			t.Fatalf("%s table differs from registered metrics", name)
		}
		for _, spec := range reportingMetrics {
			if _, ok := table[spec.definition.Id]; !ok {
				t.Fatalf("missing %s for %s", name, spec.definition.Id)
			}
			grants[spec.object] = principal.ObjectGrant{Read: true}
		}
	}
	ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, Permissions: principal.Permissions{Objects: grants}})
	evaluator := metricEvaluator{}
	original, err := evaluator.Catalog(ctx, textlang.English)
	if err != nil {
		t.Fatal(err)
	}
	if len(original.Metrics) != len(reportingMetrics) {
		t.Fatal("catalog test did not reach every metric")
	}
	for _, language := range textlang.Shipped {
		if language == textlang.English {
			continue
		}
		translated, err := evaluator.Catalog(ctx, language)
		if err != nil {
			t.Fatal(err)
		}
		for i := range translated.Metrics {
			got, want := &translated.Metrics[i], original.Metrics[i]
			for name, pair := range map[string][2]string{
				"definition":  {got.Definition, want.Definition},
				"attribution": {*got.Attribution, *want.Attribution},
				"incomplete":  {*got.IncompletePolicy, *want.IncompletePolicy},
			} {
				if pair[0] == "" || pair[0] == pair[1] {
					t.Fatalf("%s serves an untranslated %s for %s", language, name, got.Id)
				}
			}
			got.Definition, got.Attribution, got.IncompletePolicy = want.Definition, want.Attribution, want.IncompletePolicy
		}
		if !reflect.DeepEqual(translated, original) {
			t.Fatalf("%s changed metric facts or supported operations", language)
		}
	}
	denied, err := evaluator.Catalog(context.Background(), textlang.German)
	if err != nil || len(denied.Metrics) != 0 {
		t.Fatalf("catalog leaked inaccessible metrics: %+v %v", denied, err)
	}
}
