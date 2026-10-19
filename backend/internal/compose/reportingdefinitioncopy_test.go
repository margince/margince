// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestReportingCatalogTranslationsCoverExactlyTheRegisteredMetrics(t *testing.T) {
	langcopytest.Census(t, reportingDefinitionWords)
	if len(reportingDefinitionWords) != len(reportingMetrics) {
		t.Fatal("definition table differs from registered metrics")
	}
	grants := map[string]principal.ObjectGrant{}
	for _, spec := range reportingMetrics {
		if _, ok := reportingDefinitionWords[spec.definition.Id]; !ok {
			t.Fatalf("missing definition for %s", spec.definition.Id)
		}
		grants[spec.object] = principal.ObjectGrant{Read: true}
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
	for _, language := range []textlang.Lang{textlang.German, textlang.Vietnamese} {
		translated, err := evaluator.Catalog(ctx, language)
		if err != nil {
			t.Fatal(err)
		}
		for i := range translated.Metrics {
			if translated.Metrics[i].Definition == "" || translated.Metrics[i].Definition == original.Metrics[i].Definition {
				t.Fatalf("%s missing translation for %s", language, translated.Metrics[i].Id)
			}
			translated.Metrics[i].Definition = original.Metrics[i].Definition
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
