// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
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

// A rep reads the targets that measure them: their own, their team's and the
// company's. Another rep's target and another team's stay out of the list and
// answer a read by id as absent, and so does a teammate's. An allocation sums
// colleagues' targets, so the rep gets none, while a seat that reads every owner
// target does.
func TestReportingRepReadsOwnTeamAndCompanyTargets(t *testing.T) {
	e := setupForecast(t)
	at := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	service := newReportingService(e.Pool, func() time.Time { return at })
	writer := reportingActor(e)
	teammate := reportingTeammate(t, e)
	created := map[string]crmcontracts.ReportingTarget{}
	for name, scope := range map[string]crmcontracts.ReportingScope{
		"own":        {Kind: "owner", Id: ptrUUID(e.Rep1)},
		"teammate":   {Kind: "owner", Id: ptrUUID(teammate)},
		"other rep":  {Kind: "owner", Id: ptrUUID(e.Rep3)},
		"own team":   {Kind: "team", Id: ptrUUID(e.Team1)},
		"other team": {Kind: "team", Id: ptrUUID(e.Team2)},
		"company":    {Kind: "workspace"},
	} {
		target, err := service.CreateTarget(writer, crmcontracts.ReportingTargetInput{Metric: "meetings_held", Scope: scope, PeriodKind: "month", PeriodStart: openapi_types.Date{Time: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}, Value: 15, Reason: "Month plan"})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		created[name] = target
	}
	rep := reportingRepSeat(e)
	list, err := service.ListTargets(rep, nil, 100, reporting.TargetFilter{})
	if err != nil {
		t.Fatal(err)
	}
	listed := map[openapi_types.UUID]bool{}
	for _, target := range list.Data {
		listed[target.Id] = true
	}
	for name, want := range map[string]bool{"own": true, "own team": true, "company": true, "teammate": false, "other rep": false, "other team": false} {
		if listed[created[name].Id] != want {
			t.Errorf("%s target listed = %v, want %v", name, !want, want)
		}
		read, err := service.GetTarget(rep, ids.UUID(created[name].Id))
		if want && err != nil {
			t.Errorf("%s target read by id: %v", name, err)
		}
		if read.AllocatedValue != nil || read.AllocationDifference != nil {
			t.Errorf("%s target shows the rep an allocation: %+v", name, read)
		}
		if !want && !errors.Is(err, apperrors.ErrNotFound) {
			t.Errorf("%s target read by id = %v, want not found", name, err)
		}
	}
	for _, target := range list.Data {
		if target.AllocatedValue != nil || target.AllocationDifference != nil {
			t.Errorf("listed target shows the rep an allocation: %+v", target)
		}
	}
	company, err := service.GetTarget(writer, ids.UUID(created["company"].Id))
	if err != nil {
		t.Fatal(err)
	}
	if company.AllocatedValue == nil || *company.AllocatedValue != 45 || company.AllocationDifference == nil {
		t.Fatalf("a seat reading every owner target lost the allocation: %+v", company)
	}
	// The evaluation holds a figure against a target only when the reader may
	// read that target, so naming another team does not reveal its target.
	other := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(e.Team2)}, Period: "this_month", TargetBasis: "month", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{"meetings_held"}, Blocks: []crmcontracts.ReportingBlockKind{}}
	for _, tc := range []struct {
		name   string
		reader context.Context
		want   bool
	}{{"writer", writer, true}, {"rep", rep, false}} {
		result, err := service.Evaluate(tc.reader, other)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := len(result.Metrics) == 1 && result.Metrics[0].Target != nil; got != tc.want {
			t.Errorf("%s sees another team's target = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// The attainment chart of a team sums its members' owner targets. A rep in the
// team cannot read their colleagues' targets, so the chart gives them no sum.
func TestReportingNarrowSeatGetsNoChartAllocation(t *testing.T) {
	e := setupForecast(t)
	at := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	service := newReportingService(e.Pool, func() time.Time { return at })
	writer := reportingActor(e)
	teammate := reportingTeammate(t, e)
	for _, scope := range []crmcontracts.ReportingScope{{Kind: "team", Id: ptrUUID(e.Team1)}, {Kind: "owner", Id: ptrUUID(e.Rep1)}, {Kind: "owner", Id: ptrUUID(teammate)}} {
		if _, err := service.CreateTarget(writer, crmcontracts.ReportingTargetInput{Metric: "bookings_won", Scope: scope, PeriodKind: "month", PeriodStart: openapi_types.Date{Time: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}, Value: 1000000, Reason: "Month plan"}); err != nil {
			t.Fatal(err)
		}
	}
	selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(e.Team1)}, Period: "this_month", TargetBasis: "month", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{"bookings_won"}, Blocks: []crmcontracts.ReportingBlockKind{"owner_attainment"}}
	for _, tc := range []struct {
		name   string
		reader context.Context
		want   bool
	}{{"writer", writer, true}, {"rep", reportingRepSeat(e), false}} {
		result, err := service.Evaluate(tc.reader, selection)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if len(result.Charts) != 1 {
			t.Fatalf("%s: charts %+v", tc.name, result.Charts)
		}
		chart := result.Charts[0]
		if got := chart.AllocatedTarget != nil && chart.AllocationDifference != nil; got != tc.want {
			t.Errorf("%s sees the chart allocation = %v, want %v: %+v", tc.name, got, tc.want, chart)
		}
	}
}

// reportingTeammate is a second rep in Team1, beside Rep1.
func reportingTeammate(t *testing.T, e *forecastEnv) ids.UUID {
	t.Helper()
	teammate := e.seedID(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, 'teammate@forecast.test', 'Teammate')`)
	if _, err := e.owner.Exec(context.Background(), `INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)`, e.Team1, teammate); err != nil {
		t.Fatal(err)
	}
	return teammate
}

// Leading a team is the team-lead grant plus membership of that team, so a lead
// revises their team's target and sees its allocation. The company's sums owner
// targets beyond the lead's teams and stays without one, and another team is
// beyond their reach.
func TestReportingTeamLeadRevisesLedTeamTarget(t *testing.T) {
	e := setupForecast(t)
	at := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	service := newReportingService(e.Pool, func() time.Time { return at })
	lead := e.seedID(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, 'lead@forecast.test', 'Lead')`)
	if _, err := e.owner.Exec(context.Background(), `INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)`, e.Team1, lead); err != nil {
		t.Fatal(err)
	}
	seat := reportingLeadSeat(e, lead)
	input := crmcontracts.ReportingTargetInput{Metric: "meetings_held", Scope: crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(e.Team1)}, PeriodKind: "month", PeriodStart: openapi_types.Date{Time: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}, Value: 40, Reason: "Team plan"}
	team, err := service.CreateTarget(seat, input)
	if err != nil {
		t.Fatal(err)
	}
	member := input
	member.Scope = crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(e.Rep1)}
	member.Value = 25
	if _, err := service.CreateTarget(seat, member); err != nil {
		t.Fatal(err)
	}
	input.Value = 45
	input.Reason = "Team plan raised"
	if _, err := service.UpdateTarget(seat, ids.UUID(team.Id), team.Version, input); err != nil {
		t.Fatalf("the lead could not revise their team's target: %v", err)
	}
	read, err := service.GetTarget(seat, ids.UUID(team.Id))
	if err != nil {
		t.Fatal(err)
	}
	if read.AllocationDifference == nil || *read.AllocationDifference != 20 {
		t.Fatalf("the lead's team target lost its allocation: %+v", read)
	}
	company, err := service.CreateTarget(reportingActor(e), crmcontracts.ReportingTargetInput{Metric: "meetings_held", Scope: crmcontracts.ReportingScope{Kind: "workspace"}, PeriodKind: "month", PeriodStart: input.PeriodStart, Value: 90, Reason: "Company plan"})
	if err != nil {
		t.Fatal(err)
	}
	if read, err := service.GetTarget(seat, ids.UUID(company.Id)); err != nil || read.AllocatedValue != nil {
		t.Fatalf("company target for a lead: %+v %v", read, err)
	}
	other := input
	other.Scope = crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(e.Team2)}
	if _, err := service.CreateTarget(seat, other); err == nil {
		t.Fatal("a lead set a target for a team they do not lead")
	}
	// Team oversight reaches every team, so a seat holding it reads and revises
	// the target it set for a team it is not in.
	actor, ok := principal.Actor(seat)
	if !ok {
		t.Fatal("missing lead actor")
	}
	actor.Permissions.Objects["team_oversight"] = principal.ObjectGrant{Read: true}
	oversight := principal.WithActor(seat, actor)
	set, err := service.CreateTarget(oversight, other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetTarget(oversight, ids.UUID(set.Id)); err != nil {
		t.Fatalf("oversight cannot read the target it set: %v", err)
	}
	other.Reason = "Plan revised"
	if _, err := service.UpdateTarget(oversight, ids.UUID(set.Id), set.Version, other); err != nil {
		t.Fatalf("oversight cannot revise the target it set: %v", err)
	}
}

// reportingLeadSeat is a team lead's grid: team row scope, the team-lead grant
// and targets to set, a member of Team1 only.
func reportingLeadSeat(e *forecastEnv, lead ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	grants := map[string]principal.ObjectGrant{"sales_target": {Read: true, Create: true, Update: true}, "team_lead": {Read: true, Create: true}}
	for _, object := range []string{"report_definition", "reporting_framework", "deal", "pipeline", "installation_settings", "activity"} {
		grants[object] = principal.ObjectGrant{Read: true}
	}
	return principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + lead.String(), UserID: lead, SeatType: principal.SeatFull, TeamIDs: []ids.UUID{e.Team1}, Permissions: principal.Permissions{RowScope: principal.RowScopeTeam, Objects: grants}})
}

// reportingRepSeat is Rep1 on a rep's grid: own row scope, a member of Team1,
// and targets to read but not to set.
func reportingRepSeat(e *forecastEnv) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	grants := map[string]principal.ObjectGrant{}
	for _, object := range []string{"report_definition", "reporting_framework", "sales_target", "deal", "pipeline", "installation_settings", "activity"} {
		grants[object] = principal.ObjectGrant{Read: true}
	}
	return principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1, SeatType: principal.SeatFull, TeamIDs: []ids.UUID{e.Team1}, Permissions: principal.Permissions{RowScope: principal.RowScopeOwn, Objects: grants}})
}
