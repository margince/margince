// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
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

func TestAFailingReportIsListedUnavailableAndTheReportsAfterItStillAnswer(t *testing.T) {
	broken := registeredReport{name: "ext:demo:broken", label: "Broken", kind: extReportBreakdown, unit: "demo", population: "ext:demo:gone", by: "site"}
	healthy := registeredReport{name: "ext:demo:visits", label: "Visits", kind: extReportCount, unit: "demo", population: "ext:demo:visit"}
	secret := registeredReport{name: "ext:demo:secret", label: "Secret", kind: extReportCount, unit: "demo", population: "ext:demo:secret"}
	unserved := registeredReport{name: "ext:demo:unserved", label: "Unserved", kind: extReportCount, unit: "demo", population: "no-such-population"}
	total := 4
	run := func(_ context.Context, r registeredReport) (extensionReportAnswer, bool, error) {
		switch r.name {
		case broken.name:
			return extensionReportAnswer{}, false, errors.New("relation does not exist")
		case secret.name:
			return extensionReportAnswer{}, false, apperrors.ErrPermissionDenied
		case unserved.name:
			return extensionReportAnswer{}, false, nil
		}
		return extensionReportAnswer{Report: r, Total: &total, Groups: []extensionReportGroup{{Count: total}}}, true, nil
	}

	got := extensionReportDirectory(context.Background(), []registeredReport{broken, secret, healthy, unserved}, run)

	if len(got) != 2 {
		t.Fatalf("directory = %+v, want the broken report and the healthy one; a refused or unserved report is left out", got)
	}
	if got[0].Name != broken.name || got[0].Status != crmcontracts.ExtensionReportStatusUnavailable {
		t.Errorf("first = %s %s, want %s unavailable", got[0].Name, got[0].Status, broken.name)
	}
	if len(got[0].Groups) != 0 || got[0].Total != nil || got[0].ExcludedByPermission != nil || got[0].Withheld {
		t.Errorf("unavailable report = %+v, want no figures", got[0])
	}
	if got[0].Label != broken.label || got[0].By == nil || *got[0].By != "site" {
		t.Errorf("unavailable report = %+v, want its label and breakdown field kept so the screen can name it", got[0])
	}
	if got[1].Name != healthy.name || got[1].Status != crmcontracts.ExtensionReportStatusOk || got[1].Total == nil || *got[1].Total != total {
		t.Errorf("second = %+v, want %s ok with its total %d", got[1], healthy.name, total)
	}
}
