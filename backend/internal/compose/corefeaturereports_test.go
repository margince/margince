// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/corefeatures"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestDisabledReportDependenciesLeaveNoCatalogOrExecutionDoor(t *testing.T) {
	checked := 0
	for name, spec := range prebuiltReports {
		records := []string{string(spec.entity)}
		for _, dependencies := range []map[string]string{spec.referenceScopes, spec.filterScopes, spec.grants} {
			records = append(records, slices.Collect(maps.Values(dependencies))...)
		}
		for _, record := range records {
			feature, owned := corefeatures.RecordFeature(record)
			if !owned {
				continue
			}
			checked++
			t.Run(name+"/"+string(feature), func(t *testing.T) {
				availability := corefeatures.New([]string{string(feature)})
				catalog := reportToolCatalog(availability)
				if slices.ContainsFunc(catalog, func(entry agents.ReportCatalogEntry) bool { return entry.Report == name }) {
					t.Fatal("disabled report remains offered")
				}
				_, err := newReportEngine(nil).runSpec(availability.Context(t.Context()), name, spec, reportRequest{})
				if !errors.Is(err, apperrors.ErrNotFound) {
					t.Fatalf("disabled report reached its query: %v", err)
				}
			})
		}
	}
	if checked == 0 {
		t.Fatal("the report dependency census found no subjects")
	}
}
