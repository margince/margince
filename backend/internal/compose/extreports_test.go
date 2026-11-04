// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/pkg/extension"
)

func TestPreflightReportsRefusesWhatTheEngineCannotRun(t *testing.T) {
	unit := func(counts []extension.ReportCount, breakdowns []extension.ReportBreakdown) []extension.Extension {
		return []extension.Extension{{
			Name: "demo", RecordKinds: []extension.RecordKindDeclaration{visitKind("ext_demo")},
			ReportCounts: counts, ReportBreakdowns: breakdowns,
		}}
	}
	cases := map[string]struct {
		exts []extension.Extension
		want string
	}{
		"fine": {unit([]extension.ReportCount{{Name: "visits", Label: "Visits", Population: "visit"}},
			[]extension.ReportBreakdown{{Name: "by_region", Label: "By region", Population: "pipeline-current", By: "cf_regions"}}), ""},
		"unknown core population":                     {unit([]extension.ReportCount{{Name: "x", Label: "X", Population: "no-such-report"}}, nil), "not a core report population"},
		"a name used twice":                           {unit([]extension.ReportCount{{Name: "visits", Label: "A", Population: "visit"}, {Name: "visits", Label: "B", Population: "visit"}}, nil), "twice"},
		"a core population that is not the deal's":    {unit(nil, []extension.ReportBreakdown{{Name: "b", Label: "B", Population: "leads-by-status", By: "cf_x"}}), "only deal populations"},
		"a core breakdown that is not a custom field": {unit(nil, []extension.ReportBreakdown{{Name: "b", Label: "B", Population: "pipeline-current", By: "owner_id"}}), "not a custom field"},
	}
	for name, c := range cases {
		err := preflightReports(c.exts)
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: %v", name, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s: err = %v, want one naming %q", name, err, c.want)
		}
	}
}

func TestTheReportKindsAreTheContractsKinds(t *testing.T) {
	if crmcontracts.ExtensionReportKind(extReportCount) != crmcontracts.ExtensionReportKindCount ||
		crmcontracts.ExtensionReportKind(extReportBreakdown) != crmcontracts.ExtensionReportKindBreakdown {
		t.Errorf("the registry's kinds %q and %q drifted from the contract's %q and %q",
			extReportCount, extReportBreakdown, crmcontracts.ExtensionReportKindCount, crmcontracts.ExtensionReportKindBreakdown)
	}
}
