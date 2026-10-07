// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"net/http"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestReportingRejectsInvalidDefinitionsWithoutSavingPartialReports(t *testing.T) {
	f := reportingBusiness(t)
	cases := []struct {
		name   string
		change func(*crmcontracts.ReportingReportInput)
	}{
		{"empty name", func(in *crmcontracts.ReportingReportInput) { in.Name = " " }},
		{"empty metrics", func(in *crmcontracts.ReportingReportInput) { in.Selection.Metrics = nil }},
		{"duplicate metric", func(in *crmcontracts.ReportingReportInput) {
			in.Selection.Metrics = []crmcontracts.ReportingMetricID{"bookings_won", "bookings_won"}
		}},
		{"unknown metric", func(in *crmcontracts.ReportingReportInput) {
			in.Selection.Metrics = []crmcontracts.ReportingMetricID{"unknown"}
		}},
		{"duplicate chart", func(in *crmcontracts.ReportingReportInput) {
			in.Selection.Blocks = []crmcontracts.ReportingBlockKind{"bookings_trend", "bookings_trend"}
		}},
		{"unrelated chart", func(in *crmcontracts.ReportingReportInput) {
			in.Selection.Blocks = []crmcontracts.ReportingBlockKind{"forecast_support"}
		}},
		{"invalid close window", func(in *crmcontracts.ReportingReportInput) { in.Selection.CloseWindow = "unknown" }},
		{"invalid period", func(in *crmcontracts.ReportingReportInput) { in.Selection.Period = "unknown" }},
		{"invalid target basis", func(in *crmcontracts.ReportingReportInput) { in.Selection.TargetBasis = "unknown" }},
		{"team without identity", func(in *crmcontracts.ReportingReportInput) { in.Audience = "team" }},
		{"private with team", func(in *crmcontracts.ReportingReportInput) { in.AudienceTeamId = ptrUUID(f.env.Team1) }},
		{"company with team", func(in *crmcontracts.ReportingReportInput) {
			in.Audience = "workspace"
			in.AudienceTeamId = ptrUUID(f.env.Team1)
		}},
		{"unknown audience", func(in *crmcontracts.ReportingReportInput) { in.Audience = "unknown" }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := crmcontracts.ReportingReportInput{Name: "Review", Audience: "private", Selection: f.selection()}
			test.change(&input)
			if _, err := f.service.CreateReport(f.human, input); !errors.Is(err, apperrors.ErrInvalidArgument) {
				t.Fatalf("invalid definition: %v", err)
			}
		})
	}
	reports, err := f.service.ListReports(f.human, nil, 100, false)
	if err != nil || len(reports.Data) != 0 {
		t.Fatalf("invalid reports persisted: %+v %v", reports, err)
	}
}

func TestReportingHTTPReadGrantsAreRequiredForEverySavedResource(t *testing.T) {
	f := reportingBusiness(t)
	actor, ok := principal.Actor(f.human)
	if !ok {
		t.Fatal("missing reader")
	}
	actor.Permissions.Objects = map[string]principal.ObjectGrant{}
	denied := principal.WithActor(f.human, actor)
	handler := reportingHTTPRouter(f)
	id := ids.NewV7().String()
	paths := []string{"reports", "reports?scheduled=true", "reports/" + id, "reports/" + id + "/evaluation", "reports/" + id + "/schedules", "reports/" + id + "/editions", "reports/" + id + "/executions", "targets", "targets/" + id, "editions/" + id, "editions/" + id + "/evidence?metric=bookings_won&context_id=interval", "editions/compare?left_id=" + id + "&right_id=" + id, "executions/" + id}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			reportingRequest(denied, t, handler, http.MethodGet, path, nil, -1, http.StatusForbidden)
		})
	}
}

func TestReportingMCPReadsShareTheSavedReportAndRejectInvalidPaging(t *testing.T) {
	f := reportingBusiness(t)
	input := crmcontracts.ReportingReportInput{Name: "Team review", Audience: "team", AudienceTeamId: ptrUUID(f.env.Team1), Selection: f.selection()}
	report, err := f.service.CreateReport(f.human, input)
	if err != nil {
		t.Fatal(err)
	}
	answer, err := readReportingResult(f.human, f.service, agents.ReportingRead{Mode: "report", ID: ids.UUID(report.Id)})
	if err != nil || answer.Report == nil || answer.Report.Name != input.Name {
		t.Fatalf("tool report: %+v %v", answer, err)
	}
	list, err := readReportingResult(f.human, f.service, agents.ReportingRead{Mode: "reports"})
	if err != nil || list.Reports == nil || len(list.Reports.Data) != 1 {
		t.Fatalf("tool library: %+v %v", list, err)
	}
	catalog, err := readReportingResult(f.human, f.service, agents.ReportingRead{Mode: "catalog"})
	if err != nil || catalog.Catalog == nil || len(catalog.Catalog.Metrics) == 0 {
		t.Fatalf("tool catalog: %+v %v", catalog, err)
	}
	result, err := readReportingResult(f.human, f.service, agents.ReportingRead{Mode: "evaluate", Selection: f.selection()})
	if err != nil || result.Evaluation == nil || *result.Evaluation.Metrics[0].Value != 21600000 {
		t.Fatalf("tool evaluation: %+v %v", result, err)
	}
	invalidCursor := "not-a-cursor"
	for _, read := range []agents.ReportingRead{{Mode: "reports", Limit: -1}, {Mode: "reports", Limit: 101}, {Mode: "reports", Cursor: &invalidCursor}, {Mode: "unsupported"}} {
		if _, err := readReportingResult(f.human, f.service, read); err == nil {
			t.Fatalf("invalid tool input accepted: %+v", read)
		}
	}
}

func TestReportingHTTPMissingResourcesNeverReturnSuccessfulEmptyRecords(t *testing.T) {
	f := reportingBusiness(t)
	handler := reportingHTTPRouter(f)
	id := ids.NewV7().String()
	paths := []string{"reports/" + id, "reports/" + id + "/evaluation", "reports/" + id + "/schedules", "reports/" + id + "/editions", "reports/" + id + "/executions", "targets/" + id, "editions/" + id, "editions/" + id + "/evidence?metric=bookings_won&context_id=interval", "editions/compare?left_id=" + id + "&right_id=" + id, "executions/" + id}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			reportingRequest(f.human, t, handler, http.MethodGet, path, nil, -1, http.StatusNotFound)
		})
	}
	for _, request := range []struct{ method, path string }{
		{http.MethodPatch, "reports/" + id},
		{http.MethodDelete, "reports/" + id},
		{http.MethodPatch, "targets/" + id},
		{http.MethodPost, "reports/" + id + "/schedules"},
		{http.MethodPatch, "schedules/" + id},
		{http.MethodPost, "reports/" + id + "/editions"},
		{http.MethodPost, "executions/" + id + "/retry"},
	} {
		t.Run(request.method+request.path, func(t *testing.T) {
			reportingRequest(f.human, t, handler, request.method, request.path, []byte("{}"), 1, http.StatusNotFound)
		})
	}
}
