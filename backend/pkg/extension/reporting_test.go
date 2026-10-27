// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import "testing"

func TestReportNameNamesTheDeclaringUnit(t *testing.T) {
	if got := ReportName("visit-log", "open_visits"); got != "ext:visit-log:open_visits" {
		t.Errorf("ReportName = %q", got)
	}
}

func TestReportCountValidate(t *testing.T) {
	kinds := []RecordKindDeclaration{validKind()}
	cases := map[string]struct {
		count ReportCount
		ok    bool
	}{
		"own kind":     {ReportCount{Name: "visits", Label: "Visits", Population: "visit"}, true},
		"core":         {ReportCount{Name: "open", Label: "Open", Population: "pipeline-current"}, true},
		"bad name":     {ReportCount{Name: "Visits", Label: "Visits", Population: "visit"}, false},
		"no label":     {ReportCount{Name: "visits", Label: " ", Population: "visit"}, false},
		"foreign kind": {ReportCount{Name: "visits", Label: "Visits", Population: "invoice"}, false},
	}
	for name, c := range cases {
		if err := c.count.Validate(kinds); (err == nil) != c.ok {
			t.Errorf("%s: Validate = %v, want ok=%v", name, err, c.ok)
		}
	}
}

func TestReportBreakdownValidate(t *testing.T) {
	kinds := []RecordKindDeclaration{validKind()}
	cases := map[string]struct {
		breakdown ReportBreakdown
		ok        bool
	}{
		"declared field":   {ReportBreakdown{Name: "by_site", Label: "By site", Population: "visit", By: "site"}, true},
		"undeclared field": {ReportBreakdown{Name: "by_site", Label: "By site", Population: "visit", By: "owner"}, false},
		"core custom":      {ReportBreakdown{Name: "by_region", Label: "By region", Population: "pipeline-current", By: "cf_regions"}, true},
		"raw expression":   {ReportBreakdown{Name: "by_region", Label: "By region", Population: "pipeline-current", By: "cf_regions; DROP"}, false},
	}
	for name, c := range cases {
		if err := c.breakdown.Validate(kinds); (err == nil) != c.ok {
			t.Errorf("%s: Validate = %v, want ok=%v", name, err, c.ok)
		}
	}
}
