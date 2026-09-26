// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "testing"

// Every population that groups by a company or a project names it, whichever
// declaration — row scope, filter gate, the row's own id — says what it is.
func TestEveryCompanyAndProjectDimensionResolvesToItsRecordType(t *testing.T) {
	t.Parallel()
	want := map[string]string{
		fieldCompanyID: tableCompany, fieldPartnerCompanyID: tableCompany, fieldProjectID: tableProject,
	}
	seen := 0
	for key, spec := range prebuiltReports {
		for dimension, recordType := range want {
			if _, offered := spec.dimensions[dimension]; !offered {
				continue
			}
			seen++
			if got, named := groupRecordType(spec, dimension); !named || got != recordType {
				t.Errorf("%s.%s resolves to %q, want %q", key, dimension, got, recordType)
			}
		}
	}
	if seen == 0 {
		t.Fatal("no population offers a company or project dimension — this test checks nothing")
	}
}
