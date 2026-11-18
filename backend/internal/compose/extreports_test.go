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
		"a core breakdown that is not a custom field": {unit(nil, []extension.ReportBreakdown{{Name: "b", Label: "B", Population: "pipeline-current", By: "owner_id"}}), "neither a custom field"},
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

// The composition serves the reports of the units it registered, each over the
// unit's own kind, and a core population over no kind.
func TestACompositionServesItsUnitsReports(t *testing.T) {
	composed, err := RegisterExtensions(composableAll([]extension.Extension{{
		Name: "reports-demo", RecordKinds: []extension.RecordKindDeclaration{visitKind("ext_reports_demo")},
		ReportCounts: []extension.ReportCount{{Name: "visits", Label: "Visits", Population: "visit"}},
		ReportBreakdowns: []extension.ReportBreakdown{
			{Name: "by_region", Label: "By region", Population: "pipeline-current", By: "cf_regions"},
		},
	}}), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := composed.reports()
	if len(got) != 2 || got[0].name != "ext:reports-demo:by_region" || got[1].name != "ext:reports-demo:visits" {
		t.Fatalf("reports = %+v, want by_region and visits in name order", got)
	}
	if got[0].overKind != nil {
		t.Errorf("the core population resolved to kind %s", got[0].overKind.Name)
	}
	if kind := got[1].overKind; kind == nil || kind.Table != "ext_reports_demo_visit" {
		t.Errorf("the count's kind = %+v, want the unit's visit kind", kind)
	}
	if (*Extensions)(nil).reports() != nil {
		t.Error("a composition with no units served reports")
	}
}
