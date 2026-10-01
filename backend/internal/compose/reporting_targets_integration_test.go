// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestReportingTeamTargetRemainsIndependentOfOwnerAllocations(t *testing.T) {
	e := setupForecast(t)
	at := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	service := newReportingService(e.Pool, func() time.Time { return at })
	ctx := reportingActor(e)
	input := crmcontracts.ReportingTargetInput{Metric: "bookings_won", Scope: crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(e.Team1)}, PeriodKind: "month", PeriodStart: openapi_types.Date{Time: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}, Value: 30000000, Reason: "Team commitment"}
	team, err := service.CreateTarget(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	input.Scope = crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(e.Rep1)}
	input.Value = 22000000
	owner, err := service.CreateTarget(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateTarget(ctx, input); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("duplicate target: %v", err)
	}
	target, err := service.GetTarget(ctx, ids.UUID(team.Id))
	if err != nil {
		t.Fatal(err)
	}
	if target.Definition.Value != 30000000 || target.AllocatedValue == nil || *target.AllocatedValue != 22000000 || target.AllocationDifference == nil || *target.AllocationDifference != 8000000 {
		t.Fatalf("independent allocation: %+v", target)
	}
	input.Value = 35000000
	input.Reason = "Revised owner commitment"
	if _, err := service.UpdateTarget(ctx, ids.UUID(owner.Id), owner.Version, input); err != nil {
		t.Fatal(err)
	}
	list, err := service.ListTargets(ctx, nil, 100, reporting.TargetFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range list.Data {
		if target.Id != team.Id {
			continue
		}
		if target.Definition.Value != 30000000 || target.AllocationDifference == nil || *target.AllocationDifference != -5000000 {
			t.Fatalf("team target changed with allocation: %+v", target)
		}
		return
	}
	t.Fatal("team target missing from list")
}

func TestReportingRetiredTargetStopsLiveAttainmentAndKeepsRevisionHistory(t *testing.T) {
	f := reportingBusiness(t)
	target := f.target
	input := target.Definition
	before, err := f.service.Evaluate(f.human, f.selection())
	if err != nil {
		t.Fatal(err)
	}
	if before.Metrics[0].Target == nil {
		t.Fatal("active target was not applied")
	}
	report, err := f.service.CreateReport(f.human, crmcontracts.ReportingReportInput{Name: "Monthly review", Audience: "private", Selection: f.selection()})
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.service.Freeze(f.human, ids.UUID(report.Id), 0, "before-retirement")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.Sweep(principal.SystemActing(f.human, "system:retirement-test")); err != nil {
		t.Fatal(err)
	}
	completed, err := f.service.GetExecution(f.human, ids.UUID(run.Id))
	if err != nil || completed.EditionId == nil {
		t.Fatalf("publication: %+v %v", completed, err)
	}
	retired := true
	input.Retired = &retired
	input.Reason = "Incorrect allocation retired"
	updated, err := f.service.UpdateTarget(f.human, ids.UUID(target.Id), target.Version, input)
	if err != nil {
		t.Fatal(err)
	}
	after, err := f.service.Evaluate(f.human, f.selection())
	if err != nil {
		t.Fatal(err)
	}
	for _, metric := range after.Metrics {
		if metric.Id == input.Metric && metric.Target != nil {
			t.Fatal("retired target still drives live attainment")
		}
	}
	frozen, err := f.service.GetEdition(f.human, ids.UUID(*completed.EditionId))
	if err != nil {
		t.Fatal(err)
	}
	if frozen.Evaluation.Metrics[0].Target == nil || *frozen.Evaluation.Metrics[0].Target != float64(target.Definition.Value) {
		t.Fatal("retirement rewrote the saved snapshot target")
	}
	history, err := f.service.GetTarget(f.human, ids.UUID(target.Id))
	if err != nil {
		t.Fatal(err)
	}
	if history.History == nil || len(*history.History) != 2 || (*history.History)[0].Value != input.Value || (*history.History)[1].Reason != input.Reason || updated.Version != target.Version+1 {
		t.Fatal("retirement did not preserve reasoned revision history")
	}
	if _, err := f.service.UpdateTarget(f.human, ids.UUID(target.Id), target.Version, input); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("stale retirement accepted: %v", err)
	}
	input.Retired = nil
	input.Value = 0
	input.Reason = "Explicit zero allocation"
	if _, err := f.service.UpdateTarget(f.human, ids.UUID(target.Id), updated.Version, input); err != nil {
		t.Fatal(err)
	}
	zero, err := f.service.Evaluate(f.human, f.selection())
	if err != nil {
		t.Fatal(err)
	}
	if zero.Metrics[0].Target == nil || *zero.Metrics[0].Target != 0 {
		t.Fatal("explicit zero target treated as retired")
	}
}

func TestReportingPausedSchedulesRemainDiscoverableWithoutExposingSetupToReaders(t *testing.T) {
	f := reportingBusiness(t)
	report, err := f.service.CreateReport(f.human, crmcontracts.ReportingReportInput{Name: "Scheduled review", Audience: "private", Selection: f.selection()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.service.CreateSchedule(f.human, ids.UUID(report.Id), crmcontracts.ReportingScheduleInput{Frequency: "weekly", Day: 1, LocalTime: "09:00", ReportRevision: report.Revision, Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	reports, err := f.service.ListReports(f.human, nil, 100, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports.Data) != 1 || reports.Data[0].Cadence == nil || *reports.Data[0].Cadence != "weekly" || reports.Data[0].PausedScheduleCount == nil || *reports.Data[0].PausedScheduleCount != 1 || reports.Data[0].NextDueAt != nil {
		t.Fatalf("paused report summary: %+v", reports)
	}
	actor, ok := principal.Actor(f.human)
	if !ok {
		t.Fatal("missing fixture actor")
	}
	grant := actor.Permissions.Objects["report_schedule"]
	grant.Create = false
	actor.Permissions.Objects["report_schedule"] = grant
	catalog, err := f.service.Catalog(principal.WithActor(f.human, actor))
	if err != nil {
		t.Fatal(err)
	}
	if catalog.ScheduleReady == nil {
		t.Fatal("schedule updater did not receive setup readiness")
	}
	grant.Update = false
	actor.Permissions.Objects["report_schedule"] = grant
	catalog, err = f.service.Catalog(principal.WithActor(f.human, actor))
	if err != nil {
		t.Fatal(err)
	}
	if catalog.ScheduleReady != nil {
		t.Fatal("read-only schedule reader received setup readiness")
	}
}

func TestReportingTargetFiltersApplyBeforePagination(t *testing.T) {
	f := reportingBusiness(t)
	input := f.target.Definition
	input.Scope = crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(f.env.Rep1)}
	input.Reason = "Individual allocation"
	owner, err := f.service.CreateTarget(f.human, input)
	if err != nil {
		t.Fatal(err)
	}
	retired := true
	input.Retired = &retired
	input.Reason = "Allocation ended"
	if _, err := f.service.UpdateTarget(f.human, ids.UUID(owner.Id), owner.Version, input); err != nil {
		t.Fatal(err)
	}
	active := false
	handler := reportingHTTPRouter(f)
	for _, tc := range []struct {
		name   string
		filter reporting.TargetFilter
		want   int
	}{
		{"all", reporting.TargetFilter{}, 2},
		{"active", reporting.TargetFilter{Retired: &active}, 1},
		{"retired", reporting.TargetFilter{Retired: &retired}, 1},
		{"period", reporting.TargetFilter{PeriodStart: &input.PeriodStart}, 2},
		{"other period", reporting.TargetFilter{PeriodStart: &openapi_types.Date{Time: input.PeriodStart.AddDate(0, 1, 0)}}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			count := 0
			var cursor *ids.UUID
			for {
				page, err := f.service.ListTargets(f.human, cursor, 1, tc.filter)
				if err != nil {
					t.Fatal(err)
				}
				count += len(page.Data)
				if tc.filter.Retired != nil && len(page.Data) > 0 {
					got := page.Data[0].Definition.Retired != nil && *page.Data[0].Definition.Retired
					if got != *tc.filter.Retired {
						t.Fatalf("wrong retirement state: %+v", page.Data[0])
					}
				}
				if page.NextCursor == nil {
					break
				}
				next, err := ids.Parse(*page.NextCursor)
				if err != nil {
					t.Fatal(err)
				}
				cursor = &next
			}
			if count != tc.want {
				t.Fatalf("got %d targets, want %d", count, tc.want)
			}
			params := url.Values{}
			if tc.filter.Retired != nil {
				params.Set("retired", strconv.FormatBool(*tc.filter.Retired))
			}
			if tc.filter.PeriodStart != nil {
				params.Set("period_start", tc.filter.PeriodStart.Format("2006-01-02"))
			}
			response := reportingRequest(f.human, t, handler, http.MethodGet, "targets?"+params.Encode(), nil, -1, http.StatusOK)
			page := reportingResponse[crmcontracts.ReportingTargetList](t, response)
			if len(page.Data) != tc.want {
				t.Fatalf("HTTP filter returned %d targets, want %d", len(page.Data), tc.want)
			}
		})
	}
}
