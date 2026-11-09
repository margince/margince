// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"
)

func TestEveryConvertedMoneyMeasureDeclaresCoverage(t *testing.T) {
	checked := 0
	for name, spec := range prebuiltReports {
		for measure := range spec.measures {
			if !strings.HasSuffix(measure, "_minor") || spec.nativeMeasures[measure] {
				continue
			}
			checked++
			if spec.sumCoverage[measure] == "" {
				t.Errorf("%s.%s converts money without a coverage expression", name, measure)
			}
		}
		for measure, coverage := range spec.sumCoverage {
			if spec.measures[measure] == "" || coverage == "" {
				t.Errorf("%s has empty or unoffered coverage for %s", name, measure)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no converted money measures were checked")
	}
}
