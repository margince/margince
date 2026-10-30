// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"reflect"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func catalogFor(t *testing.T, objects ...string) crmcontracts.ReportingCatalog {
	t.Helper()
	grants := map[string]principal.ObjectGrant{}
	for _, object := range objects {
		grants[object] = principal.ObjectGrant{Read: true}
	}
	ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, Permissions: principal.Permissions{Objects: grants}})
	catalog, err := (metricEvaluator{}).Catalog(ctx, textlang.English)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestEveryPresetIsPublishedWholeToAReaderOfEveryMetric(t *testing.T) {
	var objects []string
	for _, spec := range reportingMetrics {
		objects = append(objects, spec.object)
	}
	catalog := catalogFor(t, objects...)
	// The catalog drops a metric or chart it cannot answer. A full reader who
	// sees less than the definition means the preset names something
	// unsupported.
	if !reflect.DeepEqual(catalog.Presets, reportingPresets) {
		t.Fatalf("published presets = %+v, want the definition %+v", catalog.Presets, reportingPresets)
	}
	seen := map[crmcontracts.ReportingPresetId]bool{}
	for _, preset := range reportingPresets {
		if seen[preset.Id] || !preset.Id.Valid() {
			t.Errorf("preset %q is repeated or not in the contract", preset.Id)
		}
		seen[preset.Id] = true
	}
}

func TestAPresetKeepsOnlyTheMetricsAndChartsTheReaderMayRead(t *testing.T) {
	catalog := catalogFor(t, "activity")
	meetings := []crmcontracts.ReportingMetricID{reportingMeetingsHeld}
	blocks := []crmcontracts.ReportingBlockKind{reportingSdrOutcomes, reportingTargetProgress}
	want := []crmcontracts.ReportingPreset{
		{Id: crmcontracts.ReportingPresetIdSdr, Metrics: meetings, Blocks: blocks},
		{Id: crmcontracts.ReportingPresetIdTeam, Metrics: meetings, Blocks: blocks},
	}
	if !reflect.DeepEqual(catalog.Presets, want) {
		t.Fatalf("an activity reader's presets = %+v, want %+v", catalog.Presets, want)
	}
	catalog = catalogFor(t, reportingCreditObject)
	for _, preset := range catalog.Presets {
		if !slices.Equal(preset.Metrics, []crmcontracts.ReportingMetricID{reportingAcceptedOpportunities}) {
			t.Fatalf("a credit reader's presets = %+v", catalog.Presets)
		}
	}
	if len(catalog.Presets) != 2 {
		t.Fatalf("a credit reader is offered %d presets, want sdr and team", len(catalog.Presets))
	}
	if none := catalogFor(t); len(none.Presets) != 0 || none.Presets == nil {
		t.Fatalf("a reader of no metric gets presets %+v, want an empty list", none.Presets)
	}
}
